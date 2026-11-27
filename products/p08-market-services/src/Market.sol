// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {LabToken} from "./LabToken.sol";

contract Market {
    error WrongChain();
    error Forbidden();
    error InvalidObservation();
    error InvalidPrice();
    error InvalidQuote();
    error InvalidTerms();
    error Limit();
    error PositionChanged();

    uint256 public constant CHAIN_ID = 8283;
    uint256 public constant PRICE_SCALE = 1_000_000;
    uint256 public constant BASE_SCALE = 100_000_000;
    uint256 public constant FRESHNESS = 120;
    uint256 public constant QUOTE_TTL = 60;
    uint256 public constant FUNDING_INTERVAL = 300;
    uint256 public constant FUNDING_BPS = 10;
    uint256 public constant MAINTENANCE_BPS = 1000;
    uint256 public constant SWAP_FEE_BPS = 30;
    uint256 private constant BPS = 10_000;
    uint256 private constant MAX_UNITS = 1e24;
    uint256 private constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    bytes32 private constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)");
    bytes32 private constant QUOTE_TYPEHASH = keccak256(
        "Quote(address user,uint8 action,uint256 a,uint256 b,uint256 minA,uint256 minB,uint256 priceVersion,uint256 nonce,uint256 deadline)"
    );

    address public immutable admin;
    address public immutable quoteSigner;
    address public immutable publisherA;
    address public immutable publisherB;
    address public immutable keeper;
    LabToken public immutable base;
    LabToken public immutable quoteToken;
    bool public paused;

    uint256 public baseReserve;
    uint256 public quoteReserve;
    uint256 public totalShares;
    mapping(address user => uint256) public shares;

    struct Position {
        address owner;
        bool long;
        bool closed;
        uint256 initialMargin;
        uint256 margin;
        uint256 size;
        uint256 entryPrice;
        uint256 revision;
        uint256 nextPeriod;
    }

    uint256 public riskReserve;
    uint256 public lockedRisk;
    uint256 public nextPosition = 1;
    mapping(uint256 id => Position) public positions;

    struct Observation {
        uint256 price;
        uint256 observedAt;
    }

    Observation[2] public observations;
    uint256 public priceVersion;
    mapping(address user => mapping(uint256 nonce => bool)) public used;

    enum Action {
        SwapBase,
        SwapQuote,
        AddLiquidity,
        RemoveLiquidity,
        FxBase,
        FxQuote,
        OpenLong,
        OpenShort,
        Close
    }

    struct Quote {
        address user;
        Action action;
        uint256 a;
        uint256 b;
        uint256 minA;
        uint256 minB;
        uint256 priceVersion;
        uint256 nonce;
        uint256 deadline;
    }

    event PriceObserved(uint256 indexed version, address indexed publisher, uint256 price, uint256 observedAt);
    event PauseChanged(bool paused);
    event Executed(bytes32 indexed digest, address indexed user, Action indexed action, uint256 first, uint256 second);

    event LiquidityAdded(
        bytes32 indexed digest, address indexed user, uint256 baseUnits, uint256 quoteUnits, uint256 sharesMinted
    );

    event PositionOpened(
        uint256 indexed id,
        address indexed owner,
        bool long,
        uint256 margin,
        uint256 size,
        uint256 entryPrice,
        uint256 nextPeriod
    );
    event PositionFunded(
        uint256 indexed id, uint256 indexed period, uint256 revision, uint256 version, int256 payment, uint256 margin
    );
    event PositionClosed(
        uint256 indexed id,
        bool indexed liquidated,
        uint256 revision,
        uint256 version,
        uint256 payout,
        uint256 fundingGap
    );

    constructor(address admin_, address signer_, address publisherA_, address publisherB_, address keeper_) {
        if (block.chainid != CHAIN_ID) revert WrongChain();
        address[5] memory roles = [admin_, signer_, publisherA_, publisherB_, keeper_];
        for (uint256 i; i < roles.length; ++i) {
            if (roles[i] == address(0)) revert Forbidden();
            for (uint256 j; j < i; ++j) {
                if (roles[i] == roles[j]) revert Forbidden();
            }
        }
        admin = admin_;
        quoteSigner = signer_;
        publisherA = publisherA_;
        publisherB = publisherB_;
        keeper = keeper_;
        base = new LabToken("NU54 Market Test Base", "LAB", 8, admin_);
        quoteToken = new LabToken("NU54 Market Test Quote", "LAQ", 6, admin_);
    }

    modifier testChain() {
        if (block.chainid != CHAIN_ID) revert WrongChain();
        _;
    }

    function setPaused(bool value) external testChain {
        if (msg.sender != admin) revert Forbidden();
        paused = value;
        emit PauseChanged(value);
    }

    function observe(uint256 price, uint256 timestamp) external testChain {
        uint256 slot;
        if (msg.sender == publisherB) slot = 1;
        else if (msg.sender != publisherA) revert Forbidden();
        if (
            price == 0 || price > 100 * PRICE_SCALE || timestamp > block.timestamp
                || block.timestamp - timestamp > FRESHNESS || timestamp <= observations[slot].observedAt
        ) revert InvalidObservation();
        observations[slot] = Observation(price, timestamp);
        emit PriceObserved(++priceVersion, msg.sender, price, timestamp);
    }

    function validPrice() public view testChain returns (uint256 version, uint256 price) {
        Observation memory first = observations[0];
        Observation memory second = observations[1];
        if (
            first.observedAt == 0 || second.observedAt == 0 || first.observedAt > block.timestamp
                || second.observedAt > block.timestamp || block.timestamp - first.observedAt > FRESHNESS
                || block.timestamp - second.observedAt > FRESHNESS
        ) revert InvalidPrice();
        uint256 lower = first.price < second.price ? first.price : second.price;
        uint256 higher = first.price > second.price ? first.price : second.price;
        if ((higher - lower) * BPS > lower * 100) revert InvalidPrice();
        return (priceVersion, (lower + higher) / 2);
    }

    function quoteDigest(Quote calldata order) public view testChain returns (bytes32) {
        bytes32 domain = keccak256(
            abi.encode(
                DOMAIN_TYPEHASH,
                keccak256("NU54 Isolated Market"),
                keccak256("1"),
                block.chainid,
                address(this),
                address(this).codehash
            )
        );
        return keccak256(abi.encodePacked(hex"1901", domain, keccak256(abi.encode(QUOTE_TYPEHASH, order))));
    }

    /// @notice Estimate output at the current oracle and reserve snapshot before wallet approval.
    function preview(Quote calldata order) external view testChain returns (uint256 first, uint256 second) {
        (uint256 version, uint256 price) = validPrice();
        if (paused || version != order.priceVersion || order.a == 0 || order.a > MAX_UNITS || order.b > MAX_UNITS) {
            revert InvalidQuote();
        }
        if (order.action == Action.AddLiquidity) {
            (first,,) = _liquidityIn(order, price);
        } else if (order.action == Action.RemoveLiquidity) {
            (first, second) = _liquidityOut(order, order.user);
        } else if (order.action == Action.FxBase || order.action == Action.FxQuote) {
            first = _fx(order, price);
        } else if (order.action == Action.SwapBase || order.action == Action.SwapQuote) {
            first = _swapOut(order, price);
        } else if (order.action == Action.OpenLong || order.action == Action.OpenShort) {
            _validateOpen(order, price);
            first = order.a;
            second = order.b;
        } else {
            Position storage pos = _position(order.a, order.b);
            if (pos.owner != order.user) revert Forbidden();
            if (order.minB != 0) revert InvalidTerms();
            first = _equity(pos, price);
            if (first < order.minA) revert Limit();
        }
    }

    function execute(Quote calldata order, bytes calldata signature)
        external
        testChain
        returns (uint256 first, uint256 second)
    {
        (bytes32 digest, uint256 price) = _authorize(order, signature);
        (first, second) = _trade(order, price, digest);
        emit Executed(digest, msg.sender, order.action, first, second);
    }

    function _trade(Quote calldata order, uint256 price, bytes32 digest) private returns (uint256, uint256) {
        if (order.action == Action.OpenLong || order.action == Action.OpenShort) return (_open(order, price), 0);
        if (order.action == Action.Close) return (_manualClose(order, price), 0);
        if (order.action == Action.AddLiquidity) return (_addLiquidity(order, price, digest), 0);
        if (order.action == Action.RemoveLiquidity) return _removeLiquidity(order);
        if (order.action == Action.FxBase || order.action == Action.FxQuote) return (_fx(order, price), 0);
        if (order.action == Action.SwapBase || order.action == Action.SwapQuote) return (_swap(order, price), 0);
        revert InvalidTerms();
    }

    function _addLiquidity(Quote calldata order, uint256 price, bytes32 digest) private returns (uint256 minted) {
        uint256 baseIn;
        uint256 quoteIn;
        (minted, baseIn, quoteIn) = _liquidityIn(order, price);
        baseReserve = _cap(baseReserve + baseIn);
        quoteReserve = _cap(quoteReserve + quoteIn);
        totalShares = _cap(totalShares + minted);
        shares[msg.sender] += minted;
        base.transferFrom(msg.sender, address(this), baseIn);
        quoteToken.transferFrom(msg.sender, address(this), quoteIn);
        emit LiquidityAdded(digest, msg.sender, baseIn, quoteIn, minted);
    }

    function _liquidityIn(Quote calldata order, uint256 price)
        private
        view
        returns (uint256 minted, uint256 baseIn, uint256 quoteIn)
    {
        if (order.b == 0 || order.minA == 0 || order.minB != 0) revert InvalidTerms();
        baseIn = order.a;
        quoteIn = order.b;
        if (totalShares == 0) {
            minted = baseIn;
        } else {
            uint256 fromBase = baseIn * totalShares / baseReserve;
            uint256 fromQuote = quoteIn * totalShares / quoteReserve;
            minted = fromBase < fromQuote ? fromBase : fromQuote;
            baseIn = _ceilDiv(minted * baseReserve, totalShares);
            quoteIn = _ceilDiv(minted * quoteReserve, totalShares);
        }
        if (minted < order.minA) revert Limit();
        _checkRate((quoteReserve + quoteIn) * BASE_SCALE, (baseReserve + baseIn) * price);
        _cap(baseReserve + baseIn);
        _cap(quoteReserve + quoteIn);
        _cap(totalShares + minted);
    }

    function _removeLiquidity(Quote calldata order) private returns (uint256 baseOut, uint256 quoteOut) {
        (baseOut, quoteOut) = _liquidityOut(order, msg.sender);
        shares[msg.sender] -= order.a;
        totalShares -= order.a;
        baseReserve -= baseOut;
        quoteReserve -= quoteOut;
        base.transfer(msg.sender, baseOut);
        quoteToken.transfer(msg.sender, quoteOut);
    }

    function _liquidityOut(Quote calldata order, address user)
        private
        view
        returns (uint256 baseOut, uint256 quoteOut)
    {
        if (order.b != 0 || order.minA == 0 || order.minB == 0 || order.a > shares[user]) revert InvalidTerms();
        baseOut = order.a * baseReserve / totalShares;
        quoteOut = order.a * quoteReserve / totalShares;
        if (baseOut < order.minA || quoteOut < order.minB) revert Limit();
    }

    function _fx(Quote calldata order, uint256 price) private pure returns (uint256 out) {
        if (order.b != 0 || order.minB != 0 || order.minA == 0) revert InvalidTerms();
        out = order.action == Action.FxBase ? order.a * price / BASE_SCALE : order.a * BASE_SCALE / price;
        if (out < order.minA) revert Limit();
    }

    function _swap(Quote calldata order, uint256 price) private returns (uint256 out) {
        out = _swapOut(order, price);
        bool sellBase = order.action == Action.SwapBase;
        if (sellBase) {
            baseReserve = _cap(baseReserve + order.a);
            quoteReserve -= out;
            base.transferFrom(msg.sender, address(this), order.a);
            quoteToken.transfer(msg.sender, out);
        } else {
            quoteReserve = _cap(quoteReserve + order.a);
            baseReserve -= out;
            quoteToken.transferFrom(msg.sender, address(this), order.a);
            base.transfer(msg.sender, out);
        }
    }

    function _swapOut(Quote calldata order, uint256 price) private view returns (uint256 out) {
        if (order.b != 0 || order.minB != 0 || order.minA == 0) revert InvalidTerms();
        if (totalShares == 0) revert InvalidTerms();
        bool sellBase = order.action == Action.SwapBase;
        uint256 reserveIn = sellBase ? baseReserve : quoteReserve;
        uint256 reserveOut = sellBase ? quoteReserve : baseReserve;
        uint256 afterFee = order.a * (BPS - SWAP_FEE_BPS);
        out = afterFee * reserveOut / (reserveIn * BPS + afterFee);
        if (out < order.minA) revert Limit();
        _cap(reserveIn + order.a);
        if (sellBase) _checkRate(out * BASE_SCALE, order.a * price);
        else _checkRate(out * price, order.a * BASE_SCALE);
    }

    function seedRisk(uint256 amount) external testChain {
        if (msg.sender != admin) revert Forbidden();
        if (amount == 0) revert InvalidTerms();
        riskReserve = _cap(riskReserve + amount);
        quoteToken.transferFrom(msg.sender, address(this), amount);
    }

    function _open(Quote calldata order, uint256 price) private returns (uint256 id) {
        _validateOpen(order, price);
        lockedRisk += order.a * 2;
        riskReserve += order.a;
        id = nextPosition++;
        bool long = order.action == Action.OpenLong;
        uint256 nextPeriod = block.timestamp / FUNDING_INTERVAL + 1;
        positions[id] = Position(msg.sender, long, false, order.a, order.a, order.b, price, 1, nextPeriod);
        quoteToken.transferFrom(msg.sender, address(this), order.a);
        emit PositionOpened(id, msg.sender, long, order.a, order.b, price, nextPeriod);
    }

    function _validateOpen(Quote calldata order, uint256 price) private view {
        uint256 notional = order.b * price / BASE_SCALE;
        if (order.minA != 0 || order.minB != 0 || order.a < 1_000_000 || notional < 1_000_000 || notional > order.a) {
            revert InvalidTerms();
        }
        if (riskReserve < lockedRisk + order.a) revert Limit();
    }

    function _manualClose(Quote calldata order, uint256 price) private returns (uint256 payout) {
        Position storage pos = _position(order.a, order.b);
        if (pos.owner != msg.sender) revert Forbidden();
        if (order.minB != 0) revert InvalidTerms();
        payout = _equity(pos, price);
        if (payout < order.minA) revert Limit();
        _close(order.a, pos, false, order.priceVersion, payout);
    }

    function fund(uint256 id, uint256 revision, uint256 period, uint256 version) external testChain {
        if (msg.sender != keeper) revert Forbidden();
        Position storage pos = _position(id, revision);
        uint256 price = _keeperPrice(version);
        if (
            period != pos.nextPeriod || period != block.timestamp / FUNDING_INTERVAL
                || block.timestamp % FUNDING_INTERVAL >= FRESHNESS
                || observations[0].observedAt / FUNDING_INTERVAL != period
                || observations[1].observedAt / FUNDING_INTERVAL != period
        ) revert InvalidTerms();

        uint256 rate = (pos.size * price / BASE_SCALE) * FUNDING_BPS / BPS;
        uint256 available = pos.long ? pos.margin : pos.initialMargin * 2 - pos.margin;
        uint256 moved = rate < available ? rate : available;
        if (moved == 0) revert Limit();
        int256 payment = pos.long ? -int256(moved) : int256(moved);
        pos.margin = pos.long ? pos.margin - moved : pos.margin + moved;
        pos.nextPeriod++;
        pos.revision++;
        emit PositionFunded(id, period, pos.revision, version, payment, pos.margin);
    }

    function liquidate(uint256 id, uint256 revision, uint256 version) external testChain {
        if (msg.sender != keeper) revert Forbidden();
        Position storage pos = _position(id, revision);
        uint256 price = _keeperPrice(version);
        uint256 payout = _equity(pos, price);
        uint256 maintenance = (pos.size * price / BASE_SCALE) * MAINTENANCE_BPS / BPS;
        if (payout > maintenance) revert Limit();
        _close(id, pos, true, version, payout);
    }

    function equity(uint256 id) external view testChain returns (uint256, uint256) {
        Position storage pos = positions[id];
        if (pos.owner == address(0) || pos.closed) revert PositionChanged();
        (, uint256 price) = validPrice();
        return (_equity(pos, price), (pos.size * price / BASE_SCALE) * MAINTENANCE_BPS / BPS);
    }

    function _equity(Position storage pos, uint256 price) private view returns (uint256) {
        int256 change = int256(price) - int256(pos.entryPrice);
        int256 pnl = int256(pos.size) * (pos.long ? change : -change) / int256(BASE_SCALE);
        int256 balance = int256(pos.margin) + pnl;
        uint256 maximum = pos.initialMargin * 2;
        if (balance <= 0) return 0;
        return uint256(balance) > maximum ? maximum : uint256(balance);
    }

    function _position(uint256 id, uint256 revision) private view returns (Position storage pos) {
        pos = positions[id];
        if (pos.owner == address(0) || pos.closed || revision != pos.revision) revert PositionChanged();
    }

    function _keeperPrice(uint256 version) private view returns (uint256 price) {
        if (paused) revert InvalidPrice();
        (uint256 actual, uint256 current) = validPrice();
        if (actual != version) revert InvalidPrice();
        return current;
    }

    function _close(uint256 id, Position storage pos, bool liquidated, uint256 version, uint256 payout) private {
        pos.closed = true;
        pos.revision++;
        lockedRisk -= pos.initialMargin * 2;
        riskReserve -= payout;
        uint256 period = block.timestamp / FUNDING_INTERVAL;
        uint256 fundingGap = pos.nextPeriod <= period ? period - pos.nextPeriod + 1 : 0;
        if (payout != 0) quoteToken.transfer(pos.owner, payout);
        emit PositionClosed(id, liquidated, pos.revision, version, payout, fundingGap);
    }

    function _cap(uint256 units) private pure returns (uint256) {
        if (units > MAX_UNITS) revert Limit();
        return units;
    }

    function _ceilDiv(uint256 a, uint256 b) private pure returns (uint256) {
        return a == 0 ? 0 : (a - 1) / b + 1;
    }

    function _checkRate(uint256 actual, uint256 referenceRate) private pure {
        if (actual * BPS < referenceRate * 9_500 || actual * BPS > referenceRate * 10_500) revert Limit();
    }

    function _authorize(Quote calldata order, bytes calldata signature)
        private
        returns (bytes32 digest, uint256 price)
    {
        if (
            paused || order.user != msg.sender || used[msg.sender][order.nonce] || block.timestamp > order.deadline
                || order.deadline - block.timestamp > QUOTE_TTL || order.a == 0 || order.a > MAX_UNITS
                || order.b > MAX_UNITS
        ) revert InvalidQuote();
        (uint256 version, uint256 currentPrice) = validPrice();
        if (order.priceVersion != version) revert InvalidQuote();
        digest = quoteDigest(order);
        if (signature.length != 65) revert InvalidQuote();
        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly ("memory-safe") {
            r := calldataload(signature.offset)
            s := calldataload(add(signature.offset, 32))
            v := byte(0, calldataload(add(signature.offset, 64)))
        }
        if (uint256(s) > HALF_ORDER || ecrecover(digest, v, r, s) != quoteSigner) revert InvalidQuote();
        used[msg.sender][order.nonce] = true;
        price = currentPrice;
    }
}
