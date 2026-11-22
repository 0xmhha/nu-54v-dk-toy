-- Read model only. Neither operator nor commerce roles can originate evidence.
CREATE FUNCTION ledger.rental_nonce_number(value bytea)
RETURNS numeric LANGUAGE plpgsql IMMUTABLE STRICT SET search_path=pg_catalog,pg_temp AS $$
DECLARE result numeric:=0;
BEGIN
    IF octet_length(value)<>32 THEN RETURN NULL; END IF;
    FOR i IN 0..31 LOOP
        result:=result*256+get_byte(value,i);
    END LOOP;
    RETURN result;
END $$;
REVOKE ALL ON FUNCTION ledger.rental_nonce_number(bytea) FROM PUBLIC;
CREATE TABLE ledger.rental_generation_freeze (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    freeze_ref uuid PRIMARY KEY,
    account_ref text NOT NULL CHECK(account_ref ~ '^0x[0-9a-f]{40}$'),
    withdraw_ref text NOT NULL CHECK(withdraw_ref ~ '^0x[0-9a-f]{40}$'),
    enrollment_revision text NOT NULL CHECK(enrollment_revision ~ '^[a-z0-9_-]{1,32}$'),
    enrollment_digest bytea NOT NULL CHECK(octet_length(enrollment_digest)=32),
    chain_id bigint NOT NULL CHECK(chain_id>0),
    settlement_ref text NOT NULL CHECK(settlement_ref ~ '^0x[0-9a-f]{40}$'),
    deployment_digest bytea NOT NULL CHECK(octet_length(deployment_digest)=32),
    board_measurement bytea NOT NULL CHECK(octet_length(board_measurement)=32),
    challenge bytea NOT NULL UNIQUE CHECK(octet_length(challenge)=32),
    receipt_digest bytea NOT NULL UNIQUE CHECK(octet_length(receipt_digest)=32),
    journal_digest bytea NOT NULL CHECK(octet_length(journal_digest)=32),
    first_nonce bytea NOT NULL CHECK(octet_length(first_nonce)=32),
    next_nonce bytea NOT NULL CHECK(octet_length(next_nonce)=32 AND next_nonce>=first_nonce),
    journal_entries bigint NOT NULL CHECK(journal_entries>=0),
    payment_entries bigint NOT NULL CHECK(payment_entries>=0),
    limit_entries bigint NOT NULL CHECK(limit_entries>=0),
    burned_entries bigint NOT NULL CHECK(burned_entries>=0),
    CHECK(ledger.rental_nonce_number(next_nonce)-ledger.rental_nonce_number(first_nonce)=journal_entries),
    CHECK(payment_entries::numeric+limit_entries+burned_entries=journal_entries),
    frozen_at timestamptz NOT NULL CHECK(isfinite(frozen_at)),
    fresh_until timestamptz NOT NULL CHECK(isfinite(fresh_until) AND fresh_until>frozen_at),
    UNIQUE(branch_id,device_ref,generation),
    UNIQUE(freeze_ref,journal_entries),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_enrollment
);

CREATE TABLE ledger.rental_generation_reconciliation (
    freeze_ref uuid PRIMARY KEY,
    journal_entries bigint NOT NULL,
    stream_generation uuid NOT NULL,
    finalized_height bigint NOT NULL CHECK(finalized_height>=0),
    finalized_hash bytea NOT NULL CHECK(octet_length(finalized_hash)=32),
    assessment_digest bytea NOT NULL UNIQUE CHECK(octet_length(assessment_digest)=32),
    checked_entries bigint NOT NULL CHECK(checked_entries>=0 AND checked_entries<=journal_entries),
    resolved_entries bigint NOT NULL CHECK(resolved_entries>=0 AND resolved_entries<=checked_entries),
    coverage_complete boolean NOT NULL,
    checked_at timestamptz NOT NULL CHECK(isfinite(checked_at)),
    fresh_until timestamptz NOT NULL CHECK(isfinite(fresh_until) AND fresh_until>checked_at),
    FOREIGN KEY(freeze_ref,journal_entries) REFERENCES ledger.rental_generation_freeze(freeze_ref,journal_entries)
);

REVOKE ALL ON ledger.rental_generation_freeze,ledger.rental_generation_reconciliation
    FROM PUBLIC,nu54_p05_commerce;

CREATE FUNCTION ledger.rental_generation_pending(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE fence ledger.rental_generation_freeze;
DECLARE observed_at timestamptz;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    observed_at:=clock_timestamp();
    SELECT f.* INTO fence FROM ledger.rental_generation_freeze f
      JOIN ledger.rental_enrollment e USING(branch_id,device_ref,generation)
      WHERE e.branch_id=branch AND e.device_ref=device AND e.generation=generation_
        AND e.attested_until>observed_at AND e.attested_at<=observed_at
        AND (f.account_ref,f.withdraw_ref,f.enrollment_revision,f.enrollment_digest)=
            (e.account_ref,e.withdraw_ref,e.revision,e.enrollment_digest)
        AND f.frozen_at>=e.attested_at AND f.frozen_at<=observed_at AND f.fresh_until>observed_at
        AND NOT EXISTS (SELECT FROM ledger.rental_enrollment later WHERE later.branch_id=branch
                         AND later.device_ref=device AND later.generation>generation_);
    IF NOT FOUND THEN RETURN jsonb_build_object('state','unknown'); END IF;
    PERFORM FROM ledger.rental_generation_reconciliation r WHERE r.freeze_ref=fence.freeze_ref;
    IF NOT FOUND THEN RETURN jsonb_build_object('state','frozen'); END IF;
    -- A self-consistent assessment is not external P01/P07 authority.
    -- Enable the verified projection only with both authenticated verifiers.
    RETURN jsonb_build_object('state','incomplete');
END $$;
REVOKE ALL ON FUNCTION ledger.rental_generation_pending(uuid,uuid,uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_generation_pending(uuid,uuid,uuid,uuid,bigint) TO nu54_p05_commerce;
