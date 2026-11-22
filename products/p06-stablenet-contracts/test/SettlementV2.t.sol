// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {SettlementV2} from "../src/SettlementV2.sol";
import {PaymentTypesV2 as Types} from "../src/PaymentTypesV2.sol";
import {TestUSDC} from "../src/test-token/TestUSDC.sol";

contract MutableTokenV2 {
    enum TransferMode {
        Exact,
        ReturnFalse,
        Fee
    }
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;
    TransferMode public mode;

    function setMode(TransferMode next) external {
        mode = next;
    }

    function mint(address account, uint256 amount) external {
        balanceOf[account] += amount;
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        return true;
    }

    function transfer(address recipient, uint256 amount) external returns (bool) {
        return _move(msg.sender, recipient, amount);
    }

    function transferFrom(address payer, address recipient, uint256 amount) external returns (bool) {
        allowance[payer][msg.sender] -= amount;
        return _move(payer, recipient, amount);
    }

    function _move(address payer, address recipient, uint256 amount) private returns (bool) {
        balanceOf[payer] -= amount;
        balanceOf[recipient] += mode == TransferMode.Fee ? amount - 1 : amount;
        return mode != TransferMode.ReturnFalse;
    }
}

contract SettlementV2Test is Test {
    uint256 internal constant MERCHANT_KEY = 101;
    uint256 internal constant PAYER_KEY = 102;
    uint256 internal constant REFUND_KEY = 103;
    uint256 internal constant NEXT_MERCHANT_KEY = 104;
    uint256 internal constant NEXT_PAYER_KEY = 105;
    bytes32 internal constant MERCHANT = bytes32(uint256(50));

    address internal beneficiary;
    address internal payer;
    address internal refundSigner;
    TestUSDC internal asset;
    SettlementV2 internal settlement;

    event Paid(
        bytes32 indexed merchantId,
        bytes32 indexed orderId,
        address indexed payer,
        address beneficiary,
        address asset,
        uint256 amount,
        uint256 nonce,
        bytes32 quoteDigest,
        uint8 mode,
        uint64 payerGeneration
    );

    event Refunded(
        bytes32 indexed merchantId,
        bytes32 indexed orderId,
        bytes32 indexed refundId,
        address payer,
        address asset,
        uint256 amount,
        uint8 mode
    );

    function setUp() public {
        vm.chainId(8283);
        vm.warp(1_800_000_000);
        beneficiary = makeAddr("registered-beneficiary");
        payer = vm.addr(PAYER_KEY);
        refundSigner = vm.addr(REFUND_KEY);
        asset = new TestUSDC(address(this));
        settlement = new SettlementV2(address(asset), address(this), false);
        _configure(settlement, 2);
        asset.mint(address(this), 90);
        asset.approve(address(settlement), 90);
        settlement.depositEscrow(payer, 1, 60);
        settlement.fundRefundReserve(MERCHANT, 30);
    }

    function testV2DeploymentPinsItsChainAndAsset() public {
        assertEq(settlement.asset(), address(asset));
        assertEq(settlement.governor(), address(this));
        assertFalse(settlement.directEnabled());

        vm.chainId(1);
        vm.expectRevert(SettlementV2.WrongDomain.selector);
        new SettlementV2(address(asset), address(this), false);
    }

    function testSolidityDigestMatchesIndependentSDKFixture() public {
        string memory path = "../../packages/protocol/v2/testdata/quote.vectors";
        vm.readLine(path);
        string[] memory fields = vm.split(vm.readLine(path), "|");
        assertEq(fields[0], "kiosk-selected");
        bytes memory raw = vm.parseBytes(fields[3]);
        uint256 chain;
        address recipient;
        assembly ("memory-safe") {
            chain := mload(add(raw, 32))
            recipient := mload(add(raw, 64))
        }
        assertEq(chain, block.chainid);
        vm.etch(recipient, address(settlement).code);
        bytes memory words = new bytes(raw.length - 64);
        for (uint256 i; i < words.length; ++i) {
            words[i] = raw[i + 64];
        }
        assertEq(
            SettlementV2(recipient).quoteDigest(abi.decode(words, (Types.MerchantQuote))), vm.parseBytes32(fields[7])
        );
    }

    function testEscrowCreditsExactAmountAndRefundsTenPlusTwenty() public {
        Types.MerchantQuote memory q = _quote(address(asset), 1, 1);
        bytes32 digest = settlement.quoteDigest(q);
        vm.expectEmit(true, true, true, true, address(settlement));
        emit Paid(MERCHANT, q.orderId, payer, beneficiary, address(asset), 30, 1, digest, 1, 1);
        _pay(settlement, q, MERCHANT_KEY, PAYER_KEY);
        assertEq(asset.balanceOf(beneficiary), 30);
        assertEq(settlement.escrowBalance(payer, 1), 30);
        assertEq(settlement.totalEscrow(), 30);
        assertEq(asset.balanceOf(address(settlement)), 60);

        Types.RefundAuthorization memory a = _refund(q, 10, 1);
        vm.expectEmit(true, true, true, true, address(settlement));
        emit Refunded(MERCHANT, q.orderId, a.refundId, payer, address(asset), 10, 1);
        _refundPay(settlement, a, REFUND_KEY);
        vm.expectRevert(SettlementV2.RefundAlreadyUsed.selector);
        _refundPay(settlement, a, REFUND_KEY);
        _refundPay(settlement, _refund(q, 20, 2), REFUND_KEY);
        assertEq(settlement.escrowBalance(payer, 1), 60);
        assertEq(settlement.refundReserve(MERCHANT), 0);
        assertEq(settlement.totalRefundReserve(), 0);
        assertEq(asset.balanceOf(beneficiary), 30);

        Types.RefundAuthorization memory excessive = _refund(q, 1, 3);
        vm.expectRevert(SettlementV2.RefundAbovePayment.selector);
        _refundPay(settlement, excessive, REFUND_KEY);
        assertFalse(settlement.refundIdUsed(MERCHANT, q.orderId, excessive.refundId));
    }

    function testRefundRequiresSeparateSignerAndHasIndependentReserve() public {
        Types.MerchantQuote memory q = _quote(address(asset), 1, 1);
        _pay(settlement, q, MERCHANT_KEY, PAYER_KEY);
        Types.RefundAuthorization memory a = _refund(q, 10, 1);
        vm.expectRevert(SettlementV2.InvalidSignature.selector);
        _refundPay(settlement, a, MERCHANT_KEY);
        assertEq(settlement.refundReserve(MERCHANT), 30);
        settlement.rotateRefundSigner(MERCHANT, vm.addr(123));
        vm.expectRevert(SettlementV2.InvalidSignature.selector);
        _refundPay(settlement, a, REFUND_KEY);
        a.keyVersion = 2;
        _refundPay(settlement, a, 123);
        assertEq(settlement.refundReserve(MERCHANT), 20);
    }

    function testRenterGenerationFencesPaymentAndRefundCreditsOriginalEscrow() public {
        Types.MerchantQuote memory q = _quote(address(asset), 1, 1);
        _pay(settlement, q, MERCHANT_KEY, PAYER_KEY);
        settlement.registerPayer(payer, 2, 2, makeAddr("second-withdrawal"));
        Types.MerchantQuote memory stale = _quote(address(asset), 2, 2);
        vm.expectRevert(SettlementV2.InvalidPayer.selector);
        _pay(settlement, stale, MERCHANT_KEY, PAYER_KEY);
        _refundPay(settlement, _refund(q, 10, 1), REFUND_KEY);
        assertEq(settlement.escrowBalance(payer, 1), 40);
        assertEq(settlement.escrowBalance(payer, 2), 0);
    }

    function testRejectsMerchantForgeriesDomainReplayAndPayerReplay() public {
        Types.MerchantQuote memory q = _quote(address(asset), 1, 1);
        vm.expectRevert(SettlementV2.InvalidSignature.selector);
        _pay(settlement, q, REFUND_KEY, PAYER_KEY);
        vm.expectRevert(SettlementV2.InvalidSignature.selector);
        _pay(settlement, q, MERCHANT_KEY, REFUND_KEY);
        bytes32 hash = settlement.quoteDigest(q);
        bytes memory merchantSignature = _sign(MERCHANT_KEY, hash);
        vm.expectRevert(SettlementV2.InvalidSignature.selector);
        settlement.settle(q, merchantSignature, merchantSignature);
        SettlementV2 second = new SettlementV2(address(asset), address(this), false);
        _configure(second, 2);
        bytes memory payerSignature = _sign(PAYER_KEY, settlement.approvalDigest(hash));
        vm.expectRevert(SettlementV2.InvalidSignature.selector);
        second.settle(q, merchantSignature, payerSignature);
        vm.chainId(1);
        vm.expectRevert(SettlementV2.WrongDomain.selector);
        settlement.settle(q, merchantSignature, payerSignature);
        vm.chainId(8283);
        _pay(settlement, q, MERCHANT_KEY, PAYER_KEY);
        q.nonce = 2;
        vm.expectRevert(SettlementV2.OrderAlreadyPaid.selector);
        _pay(settlement, q, MERCHANT_KEY, PAYER_KEY);
        assertFalse(settlement.merchantNonceUsed(MERCHANT, 2));
    }

    function testNonceUniquenessAcrossOrdersMerchantsAndPayers() public {
        Types.MerchantQuote memory q = _quote(address(asset), 1, 1);
        _pay(settlement, q, MERCHANT_KEY, PAYER_KEY);
        q.orderId = bytes32(uint256(2));
        address nextPayer = vm.addr(NEXT_PAYER_KEY);
        settlement.registerPayer(nextPayer, 1, 2, makeAddr("another-withdrawal"));
        q.payer = nextPayer;
        vm.expectRevert(SettlementV2.NonceUsed.selector);
        _pay(settlement, q, MERCHANT_KEY, NEXT_PAYER_KEY);

        q.merchantId = bytes32(uint256(51));
        q.payer = payer;
        settlement.registerMerchant(q.merchantId, vm.addr(MERCHANT_KEY), beneficiary, refundSigner);
        vm.expectRevert(SettlementV2.NonceUsed.selector);
        _pay(settlement, q, MERCHANT_KEY, PAYER_KEY);
        assertFalse(settlement.merchantNonceUsed(q.merchantId, q.nonce));
    }

    function testCutoverPreservesOnlySnapshottedDigestsAndEmergencyRevocationWins() public {
        Types.MerchantQuote memory before = _quote(address(asset), 1, 1);
        Types.MerchantQuote memory revoked = _quote(address(asset), 2, 2);
        Types.MerchantQuote memory backdated = _quote(address(asset), 3, 3);
        bytes32[] memory snapshot = new bytes32[](2);
        snapshot[0] = settlement.quoteDigest(before);
        snapshot[1] = settlement.quoteDigest(revoked);
        vm.warp(block.timestamp + 1);
        address nextBeneficiary = makeAddr("next-beneficiary");
        settlement.rotateMerchant(MERCHANT, vm.addr(NEXT_MERCHANT_KEY), nextBeneficiary, snapshot);
        vm.expectRevert(SettlementV2.InvalidMerchant.selector);
        _pay(settlement, backdated, MERCHANT_KEY, PAYER_KEY);

        _pay(settlement, before, MERCHANT_KEY, PAYER_KEY);
        assertEq(asset.balanceOf(beneficiary), 30);
        Types.MerchantQuote memory after_ = _quote(address(asset), 4, 4);
        after_.beneficiaryVersion = 2;
        after_.merchantKeyVersion = 2;
        after_.beneficiary = nextBeneficiary;
        _pay(settlement, after_, NEXT_MERCHANT_KEY, PAYER_KEY);
        assertEq(asset.balanceOf(nextBeneficiary), 30);

        settlement.revokeMerchantKey(MERCHANT, 1);
        vm.expectRevert(SettlementV2.InvalidMerchant.selector);
        _pay(settlement, revoked, MERCHANT_KEY, PAYER_KEY);
        assertFalse(settlement.payerNonceUsed(payer, revoked.nonce));
    }

    function testCutoverSameSecondAcceptsOnlyFrozenOriginalQuote() public {
        Types.MerchantQuote memory original = _quote(address(asset), 1, 1);
        Types.MerchantQuote memory backdated = _quote(address(asset), 2, 2);
        bytes32[] memory snapshot = new bytes32[](1);
        snapshot[0] = settlement.quoteDigest(original);
        address nextBeneficiary = makeAddr("same-second-beneficiary");
        settlement.rotateMerchant(MERCHANT, vm.addr(MERCHANT_KEY), nextBeneficiary, snapshot);
        assertEq(original.issuedAt, block.timestamp);
        vm.expectRevert(SettlementV2.InvalidMerchant.selector);
        _pay(settlement, backdated, MERCHANT_KEY, PAYER_KEY);
        _pay(settlement, original, MERCHANT_KEY, PAYER_KEY);
        assertEq(asset.balanceOf(beneficiary), 30);
        assertEq(asset.balanceOf(nextBeneficiary), 0);
    }

    function testCutoverDoesNotExtendQuoteExpiryOrPermitFutureIssuedOldVersion() public {
        Types.MerchantQuote memory expired = _quote(address(asset), 1, 1);
        Types.MerchantQuote memory future = _quote(address(asset), 2, 2);
        future.issuedAt += 2;
        bytes32[] memory snapshot = new bytes32[](2);
        snapshot[0] = settlement.quoteDigest(expired);
        snapshot[1] = settlement.quoteDigest(future);
        vm.warp(future.issuedAt - 1);
        settlement.rotateMerchant(MERCHANT, vm.addr(MERCHANT_KEY), makeAddr("new-beneficiary"), snapshot);
        vm.warp(future.issuedAt);
        vm.expectRevert(SettlementV2.InvalidMerchant.selector);
        _pay(settlement, future, MERCHANT_KEY, PAYER_KEY);
        vm.warp(expired.expiresAt);
        vm.expectRevert(SettlementV2.InvalidTime.selector);
        _pay(settlement, expired, MERCHANT_KEY, PAYER_KEY);
    }

    function testHeldTransferRollsBackTokenAndSettlementState() public {
        vm.prank(beneficiary);
        asset.setRefusalMode(true);
        uint256 next = asset.nextHeldId();
        Types.MerchantQuote memory q = _quote(address(asset), 1, 1);
        vm.expectRevert(SettlementV2.TokenBalanceMismatch.selector);
        _pay(settlement, q, MERCHANT_KEY, PAYER_KEY);
        assertEq(asset.nextHeldId(), next);
        _assertUnchanged(settlement, q);
        assertEq(asset.balanceOf(address(settlement)), 90);
        assertEq(asset.balanceOf(address(asset)), 0);
        assertEq(asset.balanceOf(beneficiary), 0);

        vm.prank(beneficiary);
        asset.setRefusalMode(false);
        _pay(settlement, q, MERCHANT_KEY, PAYER_KEY);
        assertEq(asset.balanceOf(beneficiary), 30);
    }

    function testFalseAndFeeTransfersRevertBeforeConsumingPayment() public {
        MutableTokenV2 token = new MutableTokenV2();
        SettlementV2 other = new SettlementV2(address(token), address(this), false);
        _configure(other, 2);
        token.mint(address(this), 90);
        token.approve(address(other), 90);
        other.depositEscrow(payer, 1, 60);
        other.fundRefundReserve(MERCHANT, 30);

        Types.MerchantQuote memory q = _quote(address(token), 1, 1);
        token.setMode(MutableTokenV2.TransferMode.ReturnFalse);
        vm.expectRevert(SettlementV2.TokenTransferFailed.selector);
        _pay(other, q, MERCHANT_KEY, PAYER_KEY);
        _assertUnchanged(other, q);
        assertEq(token.balanceOf(beneficiary), 0);
        token.setMode(MutableTokenV2.TransferMode.Fee);
        vm.expectRevert(SettlementV2.TokenBalanceMismatch.selector);
        _pay(other, q, MERCHANT_KEY, PAYER_KEY);
        _assertUnchanged(other, q);
        assertEq(token.balanceOf(beneficiary), 0);

        token.setMode(MutableTokenV2.TransferMode.Exact);
        _pay(other, q, MERCHANT_KEY, PAYER_KEY);
        assertEq(token.balanceOf(beneficiary), 30);
    }

    function testDirectRequiresOwnerCallerAndAllowanceAndRefundsOriginalPayer() public {
        SettlementV2 direct = new SettlementV2(address(asset), address(this), true);
        _configure(direct, 1);
        asset.mint(payer, 30);
        asset.mint(address(this), 30);
        asset.approve(address(direct), 30);
        direct.fundRefundReserve(MERCHANT, 30);
        Types.MerchantQuote memory q = _quote(address(asset), 1, 1);
        q.mode = 2;
        q.payerRole = 1;
        vm.expectRevert(SettlementV2.InvalidPayer.selector);
        _pay(direct, q, MERCHANT_KEY, PAYER_KEY);
        vm.expectRevert(SettlementV2.InvalidParameters.selector);
        _payAsPayer(direct, q);
        vm.prank(payer);
        asset.approve(address(direct), 31);
        vm.expectRevert(SettlementV2.InvalidParameters.selector);
        _payAsPayer(direct, q);
        assertFalse(direct.payerNonceUsed(payer, q.nonce));
        assertEq(asset.balanceOf(payer), 30);
        assertEq(asset.balanceOf(beneficiary), 0);

        vm.prank(payer);
        asset.approve(address(direct), 30);
        _payAsPayer(direct, q);
        assertEq(asset.balanceOf(payer), 0);
        assertEq(asset.balanceOf(beneficiary), 30);
        assertEq(asset.allowance(payer, address(direct)), 0);

        Types.RefundAuthorization memory a = _refund(q, 30, 1);
        vm.prank(payer);
        asset.setRefusalMode(true);
        vm.expectRevert(SettlementV2.TokenBalanceMismatch.selector);
        _refundPay(direct, a, REFUND_KEY);
        assertFalse(direct.refundIdUsed(MERCHANT, q.orderId, a.refundId));
        assertEq(direct.refundReserve(MERCHANT), 30);
        vm.prank(payer);
        asset.setRefusalMode(false);
        _refundPay(direct, a, REFUND_KEY);
        assertEq(asset.balanceOf(payer), 30);
        assertEq(direct.refundReserve(MERCHANT), 0);
    }

    function _assertUnchanged(SettlementV2 target, Types.MerchantQuote memory q) private view {
        (, uint256 paid,,,) = target.payments(MERCHANT, q.orderId);
        assertEq(paid, 0);
        assertEq(target.escrowBalance(payer, 1), 60);
        assertEq(target.totalEscrow(), 60);
        assertEq(target.refundReserve(MERCHANT), 30);
        assertFalse(target.merchantNonceUsed(MERCHANT, q.nonce));
        assertFalse(target.payerNonceUsed(payer, q.nonce));
    }

    function _configure(SettlementV2 target, uint8 role) private {
        target.registerMerchant(MERCHANT, vm.addr(MERCHANT_KEY), beneficiary, refundSigner);
        target.registerPayer(payer, 1, role, makeAddr("original-withdrawal"));
    }

    function _quote(address token, uint256 order, uint256 nonce) private view returns (Types.MerchantQuote memory) {
        return Types.MerchantQuote({
            quoteId: keccak256(abi.encode("quote", order)),
            tenantCommitment: keccak256("test-tenant"),
            branchCommitment: keccak256("test-branch"),
            orderId: bytes32(order),
            merchantId: MERCHANT,
            claimCommitment: keccak256("test-claim"),
            intentCommitment: keccak256("test-intent"),
            payer: payer,
            asset: token,
            beneficiary: beneficiary,
            amount: 30,
            nonce: nonce,
            merchantKeyVersion: 1,
            beneficiaryVersion: 1,
            payerGeneration: 1,
            issuedAt: uint64(block.timestamp),
            expiresAt: uint64(block.timestamp + 120),
            mode: 1,
            payerRole: 2
        });
    }

    function _refund(Types.MerchantQuote memory q, uint256 amount, uint256 id)
        private
        view
        returns (Types.RefundAuthorization memory)
    {
        return Types.RefundAuthorization(q.merchantId, q.orderId, bytes32(id), amount, 1, uint64(block.timestamp + 120));
    }

    function _pay(SettlementV2 target, Types.MerchantQuote memory q, uint256 merchantKey, uint256 payerKey) private {
        bytes32 separator = Types.domain(block.chainid, address(target));
        bytes32 digest = Types.digest(separator, keccak256(abi.encode(Types.QUOTE_TYPEHASH, q)));
        target.settle(q, _sign(merchantKey, digest), _sign(payerKey, Types.approvalDigest(separator, digest)));
    }

    function _payAsPayer(SettlementV2 target, Types.MerchantQuote memory q) private {
        bytes32 separator = Types.domain(block.chainid, address(target));
        bytes32 digest = Types.digest(separator, keccak256(abi.encode(Types.QUOTE_TYPEHASH, q)));
        bytes memory merchantSignature = _sign(MERCHANT_KEY, digest);
        bytes memory payerSignature = _sign(PAYER_KEY, Types.approvalDigest(separator, digest));
        vm.prank(payer);
        target.settle(q, merchantSignature, payerSignature);
    }

    function _refundPay(SettlementV2 target, Types.RefundAuthorization memory a, uint256 key) private {
        target.refund(
            a,
            _sign(
                key,
                Types.digest(
                    Types.domain(block.chainid, address(target)), keccak256(abi.encode(Types.REFUND_TYPEHASH, a))
                )
            )
        );
    }

    function _sign(uint256 key, bytes32 hash) private pure returns (bytes memory) {
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(key, hash);
        return abi.encodePacked(r, s, v);
    }
}
