-- Customers prove the board account that pays instead of holding a store-issued certificate. The server gives a
-- one-time challenge, the board signs it with its account key (NU54_ACCOUNT_PROOF_OP), and the proven address decides
-- what the request may do: start a cart, follow the order it paid, or hand over a ready order. Stores register no
-- phone and no board; store-issued customer certificates keep working beside this.

-- The customer subject opsd derived from one board account. A store-issued subject never appears here.
CREATE TABLE ledger.customer_account (
    actor_id uuid PRIMARY KEY,
    payer bytea NOT NULL UNIQUE CHECK (octet_length(payer)=20),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

-- A challenge is stored by its SHA-256 and accepted once, before it expires, for its branch, purpose and order.
CREATE TABLE ledger.customer_challenge (
    challenge_hash bytea PRIMARY KEY CHECK (octet_length(challenge_hash)=32),
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    purpose text NOT NULL CHECK (purpose IN ('checkout','order','pickup')),
    sale_id uuid,
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    used_at timestamptz,
    CHECK ((purpose='checkout')=(sale_id IS NULL))
);
CREATE INDEX customer_challenge_expiry ON ledger.customer_challenge(expires_at);

-- A session is stored by the SHA-256 of its random token. A checkout session covers one branch before payment; an
-- order session covers one paid order until it ends.
CREATE TABLE ledger.customer_session (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash)=32),
    branch_id uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES ledger.customer_account,
    purpose text NOT NULL CHECK (purpose IN ('checkout','order')),
    sale_id uuid,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    CHECK ((purpose='checkout')=(sale_id IS NULL)),
    FOREIGN KEY (branch_id,actor_id) REFERENCES ledger.commerce_grant ON DELETE CASCADE
);
CREATE INDEX customer_session_expiry ON ledger.customer_session(expires_at);

-- A pickup token is the QR a ready order shows at the counter, stored by its SHA-256 and accepted once.
CREATE TABLE ledger.pickup_token (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash)=32),
    branch_id uuid NOT NULL,
    sale_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    used_at timestamptz,
    used_by uuid,
    FOREIGN KEY (branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    CHECK ((used_at IS NULL)=(used_by IS NULL))
);
CREATE INDEX pickup_token_expiry ON ledger.pickup_token(expires_at);
REVOKE ALL ON ledger.customer_account,ledger.customer_challenge,ledger.customer_session,ledger.pickup_token
    FROM PUBLIC,nu54_p05_commerce;

CREATE FUNCTION ledger.issue_customer_challenge(branch_ uuid,purpose_ text,sale_ uuid,hash_ bytea,seconds_ integer)
RETURNS timestamptz LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $challenge$
DECLARE expiry timestamptz;
BEGIN
    IF seconds_ IS NULL OR seconds_ NOT BETWEEN 1 AND 300 OR NOT EXISTS (SELECT FROM ledger.branch b WHERE b.id=branch_) THEN
        RAISE EXCEPTION 'invalid challenge' USING ERRCODE='23514';
    END IF;
    -- Old rows go when new ones come, so an unauthenticated caller cannot grow the tables for long.
    DELETE FROM ledger.customer_challenge WHERE expires_at<clock_timestamp()-interval '10 minutes';
    DELETE FROM ledger.pickup_token WHERE expires_at<clock_timestamp()-interval '1 day';
    DELETE FROM ledger.customer_session WHERE expires_at<clock_timestamp()-interval '1 day';
    expiry:=clock_timestamp()+make_interval(secs=>seconds_);
    INSERT INTO ledger.customer_challenge(challenge_hash,branch_id,purpose,sale_id,expires_at)
        VALUES(hash_,branch_,purpose_,sale_,expiry);
    RETURN expiry;
END
$challenge$;

-- Accepts one signed challenge whose account opsd has already checked. A wrong account for the purpose still uses the
-- challenge up. Outcomes: ok, mismatch (the account is not the one the purpose requires), not_ready (pickup only).
CREATE FUNCTION ledger.prove_customer_account(hash_ bytea,payer_ bytea,actor_ uuid,generation_ bigint,token_ bytea,
    checkout_seconds_ integer,order_seconds_ integer,pickup_seconds_ integer)
RETURNS TABLE(outcome_ text,purpose_ text,branch_ uuid,sale_ uuid,tenant_ uuid,expires_ timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $prove$
DECLARE c ledger.customer_challenge; known ledger.customer_account; tenant uuid; expiry timestamptz; prior ledger.payer_enrollment;
BEGIN
    IF octet_length(payer_) IS DISTINCT FROM 20 OR octet_length(token_) IS DISTINCT FROM 32 OR actor_ IS NULL THEN
        RAISE EXCEPTION 'invalid proof' USING ERRCODE='23514';
    END IF;
    UPDATE ledger.customer_challenge x SET used_at=clock_timestamp()
        WHERE x.challenge_hash=hash_ AND x.used_at IS NULL AND x.expires_at>clock_timestamp() RETURNING x.* INTO c;
    IF NOT FOUND THEN RAISE EXCEPTION 'challenge unavailable' USING ERRCODE='P0502'; END IF;
    SELECT b.tenant_ref INTO STRICT tenant FROM ledger.branch b WHERE b.id=c.branch_id;

    IF c.purpose='order' AND NOT EXISTS (SELECT FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
        WHERE s.id=c.sale_id AND s.branch_id=c.branch_id AND s.state IN ('checking','paid') AND q.signed_digest IS NOT NULL AND q.payer=payer_)
    THEN
        RETURN QUERY SELECT 'mismatch',c.purpose,c.branch_id,c.sale_id,tenant,NULL::timestamptz; RETURN;
    END IF;
    IF c.purpose='pickup' THEN
        IF NOT EXISTS (SELECT FROM ledger.sale s JOIN ledger.payment p ON p.sale_id=s.id JOIN ledger.quote_work q ON q.sale_id=s.id
            WHERE s.id=c.sale_id AND s.branch_id=c.branch_id AND s.state='paid' AND q.payer=payer_
              AND p.payer='0x'||encode(payer_,'hex')) THEN
            RETURN QUERY SELECT 'mismatch',c.purpose,c.branch_id,c.sale_id,tenant,NULL::timestamptz; RETURN;
        END IF;
        IF NOT EXISTS (SELECT FROM ledger.travel_pickup_progress k WHERE k.sale_id=c.sale_id AND k.state='ready') THEN
            RETURN QUERY SELECT 'not_ready',c.purpose,c.branch_id,c.sale_id,tenant,NULL::timestamptz; RETURN;
        END IF;
        expiry:=clock_timestamp()+make_interval(secs=>pickup_seconds_);
        INSERT INTO ledger.pickup_token(token_hash,branch_id,sale_id,expires_at) VALUES(token_,c.branch_id,c.sale_id,expiry);
        RETURN QUERY SELECT 'ok',c.purpose,c.branch_id,c.sale_id,tenant,expiry; RETURN;
    END IF;

    -- The derived subject belongs to this account only, and never to a store-issued grant.
    SELECT * INTO known FROM ledger.customer_account a WHERE a.actor_id=actor_ OR a.payer=payer_ FOR UPDATE;
    IF FOUND AND (known.actor_id,known.payer) IS DISTINCT FROM (actor_,payer_) THEN
        RAISE EXCEPTION 'customer subject collision' USING ERRCODE='42501';
    END IF;
    IF NOT FOUND THEN
        IF EXISTS (SELECT FROM ledger.commerce_grant g WHERE g.actor_id=actor_) THEN
            RAISE EXCEPTION 'customer subject collision' USING ERRCODE='42501';
        END IF;
        INSERT INTO ledger.customer_account(actor_id,payer) VALUES(actor_,payer_);
    END IF;

    expiry:=clock_timestamp()+make_interval(secs=>CASE c.purpose WHEN 'checkout' THEN checkout_seconds_ ELSE order_seconds_ END);
    INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES(c.branch_id,actor_,'customer',expiry)
        ON CONFLICT (branch_id,actor_id) DO UPDATE SET valid_until=greatest(ledger.commerce_grant.valid_until,excluded.valid_until)
        WHERE ledger.commerce_grant.role_code='customer';
    IF NOT FOUND THEN RAISE EXCEPTION 'customer subject collision' USING ERRCODE='42501'; END IF;
    IF c.purpose='checkout' THEN
        IF generation_ IS NULL OR generation_<=0 THEN RAISE EXCEPTION 'board generation required' USING ERRCODE='23514'; END IF;
        SELECT * INTO prior FROM ledger.payer_enrollment e WHERE (e.branch_id,e.actor_id)=(c.branch_id,actor_) FOR UPDATE;
        IF FOUND AND prior.payer_generation<>generation_ AND ledger.payer_has_open_order(payer_) THEN
            RAISE EXCEPTION 'the account still has an order' USING ERRCODE='P1060';
        END IF;
        INSERT INTO ledger.payer_enrollment VALUES(c.branch_id,actor_,payer_,generation_,1,expiry)
            ON CONFLICT(branch_id,actor_id) DO UPDATE SET payer_generation=excluded.payer_generation,
                expires_at=greatest(ledger.payer_enrollment.expires_at,excluded.expires_at);
    END IF;
    INSERT INTO ledger.customer_session(token_hash,branch_id,actor_id,purpose,sale_id,expires_at)
        VALUES(token_,c.branch_id,actor_,c.purpose,c.sale_id,expiry);
    RETURN QUERY SELECT 'ok',c.purpose,c.branch_id,c.sale_id,tenant,expiry;
END
$prove$;

-- The live session of a token: before it expires, and for an order session only until the order ends.
CREATE FUNCTION ledger.customer_session_live(token_ bytea)
RETURNS TABLE(token_hash bytea,branch_id uuid,actor_id uuid,purpose text,sale_id uuid,tenant_ref uuid,expires_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $live$
    SELECT x.token_hash,x.branch_id,x.actor_id,x.purpose,x.sale_id,b.tenant_ref,least(x.expires_at,g.valid_until)
    FROM ledger.customer_session x JOIN ledger.branch b ON b.id=x.branch_id
    JOIN ledger.commerce_grant g ON (g.branch_id,g.actor_id)=(x.branch_id,x.actor_id) AND g.role_code='customer'
    WHERE x.token_hash=token_ AND x.expires_at>clock_timestamp() AND g.valid_until>clock_timestamp()
      AND (x.purpose='checkout' OR NOT EXISTS (SELECT FROM ledger.sale s WHERE s.id=x.sale_id AND s.state='closed'))
      AND (x.purpose='checkout' OR NOT EXISTS (SELECT FROM ledger.order_escrow e WHERE e.sale_id=x.sale_id AND e.settled))
$live$;

-- One unpaid hold per proven account, across branches. Store-issued subjects keep their old rule.
CREATE FUNCTION ledger.one_customer_hold() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $hold$
BEGIN
    IF NOT EXISTS (SELECT FROM ledger.customer_account a WHERE a.actor_id=NEW.creator) THEN RETURN NEW; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.creator::text,122));
    IF EXISTS (SELECT FROM ledger.order_checkout c JOIN ledger.sale s ON s.id=c.sale_id
        WHERE c.creator=NEW.creator AND c.sale_id<>NEW.sale_id AND c.expires_at>clock_timestamp() AND s.state IN ('preview','issued')) THEN
        RAISE EXCEPTION 'the account already holds an unpaid order' USING ERRCODE='P1060';
    END IF;
    RETURN NEW;
END
$hold$;
CREATE TRIGGER one_customer_hold BEFORE INSERT ON ledger.order_checkout FOR EACH ROW EXECUTE FUNCTION ledger.one_customer_hold();

-- Staff scan a ready order's QR on the order page they opened. The token must be this branch's, unused, unexpired and
-- for the order selected; the order must be ready. The handover is recorded through the kitchen's own change, which
-- checks the staff member's authority and queues the escrow delivery mark. Payout keeps its receipt and window rules.
CREATE FUNCTION ledger.scan_pickup_token(tenant_ uuid,branch_ uuid,actor_ uuid,role_ text,sale_ uuid,hash_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scan$
DECLARE t ledger.pickup_token; k ledger.travel_pickup_progress; revision_ bigint;
BEGIN
    SELECT * INTO t FROM ledger.pickup_token x
        WHERE x.token_hash=hash_ AND x.branch_id=branch_ AND x.used_at IS NULL AND x.expires_at>clock_timestamp() FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'pickup token unavailable' USING ERRCODE='P0502'; END IF;
    IF t.sale_id IS DISTINCT FROM sale_ THEN RAISE EXCEPTION 'pickup token of another order' USING ERRCODE='P0763'; END IF;
    SELECT * INTO k FROM ledger.travel_pickup_progress p WHERE p.sale_id=t.sale_id FOR UPDATE;
    IF k.state IS DISTINCT FROM 'ready' THEN RAISE EXCEPTION 'order is not ready' USING ERRCODE='P0761'; END IF;
    UPDATE ledger.pickup_token SET used_at=clock_timestamp(),used_by=actor_ WHERE token_hash=hash_;
    revision_:=ledger.kiosk_pickup_change(tenant_,branch_,actor_,t.sale_id,encode(substring(hash_ FROM 1 FOR 16),'hex')::uuid,
        k.revision,'collected',role_);
    RETURN jsonb_build_object('saleRef',t.sale_id,'revision',revision_,
        'pickupNo',(SELECT p.number FROM ledger.pickup p WHERE p.sale_id=t.sale_id),
        'items',coalesce((SELECT jsonb_agg(jsonb_build_object('label',x.label,'option',l.option_key,'quantity',l.quantity)
            ORDER BY l.sku COLLATE "C",l.option_key COLLATE "C")
            FROM ledger.order_line l JOIN ledger.price x ON
              (x.branch_id,x.version,x.sku,x.option_key)=(l.branch_id,l.menu_version,l.sku,l.option_key)
            WHERE l.sale_id=t.sale_id),'[]'::jsonb));
END
$scan$;

REVOKE ALL ON FUNCTION ledger.issue_customer_challenge(uuid,text,uuid,bytea,integer),
    ledger.prove_customer_account(bytea,bytea,uuid,bigint,bytea,integer,integer,integer),
    ledger.customer_session_live(bytea),ledger.one_customer_hold(),
    ledger.scan_pickup_token(uuid,uuid,uuid,text,uuid,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.issue_customer_challenge(uuid,text,uuid,bytea,integer),
    ledger.prove_customer_account(bytea,bytea,uuid,bigint,bytea,integer,integer,integer),
    ledger.customer_session_live(bytea),ledger.scan_pickup_token(uuid,uuid,uuid,text,uuid,bytea) TO nu54_p05_commerce;
