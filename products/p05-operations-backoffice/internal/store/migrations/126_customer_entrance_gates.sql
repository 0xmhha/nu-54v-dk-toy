-- Gates of the customer entrance that came out of its second and third review. No table or column is added; the
-- challenge table takes one more purpose, and holds the challenges that were used instead of those that were given.

-- A store takes customers only while its page is published (migration 123). Migration 122 gave a challenge to
-- anyone who knew a branch's reference, so a store that never published its page, or took it back, could still be
-- ordered from through a session. Published is now the gate of a new customer as well:
--   checkout  the store's page is published; any other store answers like a branch that does not exist
--   order, pickup  the order named is one of the store named, and the store's page is published or the store
--     already took payment for that order (being checked or paid): a customer who paid follows, answers and
--     collects that order even after the store took its page back
-- The proof checks the store again for a checkout (ledger.prove_customer_account below), and a checkout session
-- ends when the store takes its page back (ledger.customer_session_live below).
--
-- A fourth purpose asks about the account alone, at no store: `account`, with no branch and no order. Its proof
-- answers how many unfinished orders the account has (ledger.prove_account_orders), so a phone that kept no record
-- of them can still learn that a board must not be reset yet.
ALTER TABLE ledger.customer_challenge ALTER COLUMN branch_id DROP NOT NULL;
ALTER TABLE ledger.customer_challenge DROP CONSTRAINT customer_challenge_purpose_check, DROP CONSTRAINT customer_challenge_check;
ALTER TABLE ledger.customer_challenge
    ADD CONSTRAINT customer_challenge_purpose_check CHECK (purpose IN ('checkout','order','pickup','account')),
    ADD CONSTRAINT customer_challenge_check CHECK ((purpose IN ('checkout','account'))=(sale_id IS NULL)),
    ADD CONSTRAINT customer_challenge_account_check CHECK ((purpose='account')=(branch_id IS NULL));

-- Giving a challenge writes nothing. Migration 122 kept a row for every challenge it gave, so asking for
-- challenges, which costs the asker nothing, wrote to the database for each one. A challenge now carries its
-- purpose, its store or order and its expiry itself, under a tag only opsd can make (opsd customer_challenge.go);
-- the database is asked only whether one may be given, and remembers a challenge when its proof is taken.
DROP FUNCTION ledger.issue_customer_challenge(uuid,text,uuid,bytea,integer);

-- Whether a challenge of this purpose may be given for this store and order. It reads and writes nothing else,
-- and has one answer for a branch that does not exist, a store that is not public and an order that is not the
-- store's. opsd gives an `account` challenge without asking.
CREATE FUNCTION ledger.customer_challenge_allowed(branch_ uuid,purpose_ text,sale_ uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $allowed$
    SELECT CASE
        WHEN branch_ IS NULL THEN false
        WHEN purpose_='checkout' THEN sale_ IS NULL AND ledger.store_page_published(branch_)
        WHEN purpose_ IN ('order','pickup') THEN coalesce((SELECT ledger.store_page_published(branch_) OR s.state IN ('checking','paid')
            FROM ledger.sale s WHERE s.id=sale_ AND s.branch_id=branch_),false)
        ELSE false END
$allowed$;

-- Uses a challenge up: one row by the SHA-256 of the challenge, written when its proof is taken. It answers false
-- for a challenge that was used before or is past its time by this database's clock, and writes nothing then. The
-- caller is a function that opsd calls only for a proof whose challenge carries opsd's own tag and whose signature
-- is the account's, so a request without both writes no row here. Rows go ten minutes after their challenge ran
-- out, as before; the old sessions and pickup tokens go with them, as they did when a challenge was given.
CREATE FUNCTION ledger.use_customer_challenge(hash_ bytea,purpose_ text,branch_ uuid,sale_ uuid,expires_ timestamptz) RETURNS boolean
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $use$
BEGIN
    IF octet_length(hash_) IS DISTINCT FROM 32 OR expires_ IS NULL OR NOT isfinite(expires_) THEN
        RAISE EXCEPTION 'invalid challenge' USING ERRCODE='23514';
    END IF;
    IF expires_<=clock_timestamp() THEN RETURN false; END IF;
    DELETE FROM ledger.customer_challenge WHERE expires_at<clock_timestamp()-interval '10 minutes';
    DELETE FROM ledger.pickup_token WHERE expires_at<clock_timestamp()-interval '1 day';
    DELETE FROM ledger.customer_session WHERE expires_at<clock_timestamp()-interval '1 day';
    INSERT INTO ledger.customer_challenge(challenge_hash,branch_id,purpose,sale_id,expires_at,used_at)
        VALUES(hash_,branch_,purpose_,sale_,expires_,clock_timestamp()) ON CONFLICT (challenge_hash) DO NOTHING;
    RETURN FOUND;
END
$use$;

-- Accepts one signed challenge whose tag and whose account opsd has already checked. asked_ and subject_ are what
-- the challenge says of itself: its purpose, and the store of a checkout or the order of an order session or a
-- pickup QR; until_ is when it runs out. This is the function of migration 125 with the challenge read from its
-- arguments instead of from a row: the store of an order is the order's own, the challenge is used up here (a
-- wrong account for the purpose still uses it up), and everything after that is unchanged. Outcomes: ok, mismatch
-- (the account is not the one the purpose requires), not_ready and collected (pickup only). A challenge that was
-- used before or ran out, an order that does not exist and a checkout at a store that is not public are answered
-- alike (P0502).
DROP FUNCTION ledger.prove_customer_account(bytea,bytea,uuid,bigint,bytea,integer,integer,integer);
CREATE FUNCTION ledger.prove_customer_account(hash_ bytea,asked_ text,subject_ uuid,until_ timestamptz,payer_ bytea,actor_ uuid,
    generation_ bigint,token_ bytea,checkout_seconds_ integer,order_seconds_ integer,pickup_seconds_ integer)
RETURNS TABLE(outcome_ text,purpose_ text,branch_ uuid,sale_ uuid,tenant_ uuid,expires_ timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $prove$
DECLARE store uuid; ordered uuid; known ledger.customer_account; tenant uuid; expiry timestamptz; prior ledger.payer_enrollment;
    handed timestamptz;
BEGIN
    IF octet_length(payer_) IS DISTINCT FROM 20 OR octet_length(token_) IS DISTINCT FROM 32 OR actor_ IS NULL OR subject_ IS NULL
       OR asked_ IS NULL OR asked_ NOT IN ('checkout','order','pickup') THEN
        RAISE EXCEPTION 'invalid proof' USING ERRCODE='23514';
    END IF;
    IF asked_='checkout' THEN
        store:=subject_;
    ELSE
        ordered:=subject_;
        SELECT s.branch_id INTO store FROM ledger.sale s WHERE s.id=subject_;
    END IF;
    IF store IS NULL OR (asked_='checkout' AND NOT ledger.store_page_published(store))
       OR NOT ledger.use_customer_challenge(hash_,asked_,store,ordered,until_) THEN
        RAISE EXCEPTION 'challenge unavailable' USING ERRCODE='P0502';
    END IF;
    SELECT b.tenant_ref INTO STRICT tenant FROM ledger.branch b WHERE b.id=store;

    IF asked_='order' AND NOT EXISTS (SELECT FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
        WHERE s.id=ordered AND s.branch_id=store AND s.state IN ('checking','paid') AND q.signed_digest IS NOT NULL AND q.payer=payer_)
    THEN
        RETURN QUERY SELECT 'mismatch',asked_,store,ordered,tenant,NULL::timestamptz; RETURN;
    END IF;
    IF asked_='pickup' THEN
        IF NOT EXISTS (SELECT FROM ledger.sale s JOIN ledger.payment p ON p.sale_id=s.id JOIN ledger.quote_work q ON q.sale_id=s.id
            WHERE s.id=ordered AND s.branch_id=store AND s.state='paid' AND q.payer=payer_
              AND p.payer='0x'||encode(payer_,'hex')) THEN
            RETURN QUERY SELECT 'mismatch',asked_,store,ordered,tenant,NULL::timestamptz; RETURN;
        END IF;
        -- Only the account that paid learns that its order was handed over, and when.
        SELECT k.updated_at INTO handed FROM ledger.travel_pickup_progress k WHERE k.sale_id=ordered AND k.state='collected';
        IF FOUND THEN
            RETURN QUERY SELECT 'collected',asked_,store,ordered,tenant,handed; RETURN;
        END IF;
        IF NOT EXISTS (SELECT FROM ledger.travel_pickup_progress k WHERE k.sale_id=ordered AND k.state='ready') THEN
            RETURN QUERY SELECT 'not_ready',asked_,store,ordered,tenant,NULL::timestamptz; RETURN;
        END IF;
        expiry:=clock_timestamp()+make_interval(secs=>pickup_seconds_);
        INSERT INTO ledger.pickup_token(token_hash,branch_id,sale_id,expires_at) VALUES(token_,store,ordered,expiry);
        RETURN QUERY SELECT 'ok',asked_,store,ordered,tenant,expiry; RETURN;
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

    expiry:=clock_timestamp()+make_interval(secs=>CASE asked_ WHEN 'checkout' THEN checkout_seconds_ ELSE order_seconds_ END);
    INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES(store,actor_,'customer',expiry)
        ON CONFLICT (branch_id,actor_id) DO UPDATE SET valid_until=greatest(ledger.commerce_grant.valid_until,excluded.valid_until)
        WHERE ledger.commerce_grant.role_code='customer';
    IF NOT FOUND THEN RAISE EXCEPTION 'customer subject collision' USING ERRCODE='42501'; END IF;
    IF asked_='checkout' THEN
        IF generation_ IS NULL OR generation_<=0 THEN RAISE EXCEPTION 'board generation required' USING ERRCODE='23514'; END IF;
        SELECT * INTO prior FROM ledger.payer_enrollment e WHERE (e.branch_id,e.actor_id)=(store,actor_) FOR UPDATE;
        IF FOUND AND prior.payer_generation<>generation_ AND ledger.payer_has_open_order(payer_) THEN
            RAISE EXCEPTION 'the account still has an order' USING ERRCODE='P1060';
        END IF;
        INSERT INTO ledger.payer_enrollment VALUES(store,actor_,payer_,generation_,1,expiry)
            ON CONFLICT(branch_id,actor_id) DO UPDATE SET payer_generation=excluded.payer_generation,
                expires_at=greatest(ledger.payer_enrollment.expires_at,excluded.expires_at);
    END IF;
    INSERT INTO ledger.customer_session(token_hash,branch_id,actor_id,purpose,sale_id,expires_at)
        VALUES(token_,store,actor_,asked_,ordered,expiry);
    RETURN QUERY SELECT 'ok',asked_,store,ordered,tenant,expiry;
END
$prove$;

-- The live session of a token, as in migration 122, with one rule added: a checkout session lives only while its
-- store's page is published. An order session is untouched by that: it follows an order that was paid.
CREATE OR REPLACE FUNCTION ledger.customer_session_live(token_ bytea)
RETURNS TABLE(token_hash bytea,branch_id uuid,actor_id uuid,purpose text,sale_id uuid,tenant_ref uuid,expires_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $live$
    SELECT x.token_hash,x.branch_id,x.actor_id,x.purpose,x.sale_id,b.tenant_ref,least(x.expires_at,g.valid_until)
    FROM ledger.customer_session x JOIN ledger.branch b ON b.id=x.branch_id
    JOIN ledger.commerce_grant g ON (g.branch_id,g.actor_id)=(x.branch_id,x.actor_id) AND g.role_code='customer'
    WHERE x.token_hash=token_ AND x.expires_at>clock_timestamp() AND g.valid_until>clock_timestamp()
      AND (x.purpose='checkout' OR NOT EXISTS (SELECT FROM ledger.sale s WHERE s.id=x.sale_id AND s.state='closed'))
      AND (x.purpose='checkout' OR NOT EXISTS (SELECT FROM ledger.order_escrow e WHERE e.sale_id=x.sale_id AND e.settled))
      AND (x.purpose<>'checkout' OR ledger.store_page_published(x.branch_id))
$live$;

-- How many orders may still take or return money of this account, at any store: the orders
-- ledger.payer_has_open_order (migration 107) says yes for, counted.
CREATE FUNCTION ledger.payer_open_orders(payer_ bytea) RETURNS integer
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT count(DISTINCT s.id)::integer FROM ledger.quote_work q JOIN ledger.sale s ON s.id=q.sale_id
        LEFT JOIN ledger.order_escrow e ON e.sale_id=s.id
        WHERE q.payer=payer_ AND (s.state='checking' OR (s.state IN ('preview','issued') AND q.expires_at>clock_timestamp())
            OR (e.sale_id IS NOT NULL AND NOT e.settled))
$$;

-- Accepts one signed `account` challenge whose tag and whose account opsd has already checked, and answers the
-- number of unfinished orders of that account and nothing else: no order, no store, no amount. NULL when the
-- challenge was used before or is past its time; opsd answers that like any challenge that ran out.
CREATE FUNCTION ledger.prove_account_orders(hash_ bytea,payer_ bytea,until_ timestamptz) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $orders$
BEGIN
    IF octet_length(payer_) IS DISTINCT FROM 20 THEN
        RAISE EXCEPTION 'invalid proof' USING ERRCODE='23514';
    END IF;
    IF NOT ledger.use_customer_challenge(hash_,'account',NULL,NULL,until_) THEN RETURN NULL; END IF;
    RETURN ledger.payer_open_orders(payer_);
END
$orders$;

-- A store's registered name is shown to customers on its page, so it is one visible line like the page's own text
-- (ledger.store_text_plain, migration 123). The rule of migration 029 refused only control characters. Names saved
-- before this are not checked again; a store that saves its name anew is.
ALTER TABLE ledger.operator_branch_label ADD CONSTRAINT operator_branch_label_plain
    CHECK (ledger.store_text_plain(label)) NOT VALID;

REVOKE ALL ON FUNCTION ledger.payer_open_orders(bytea),ledger.prove_account_orders(bytea,bytea,timestamptz),
    ledger.customer_challenge_allowed(uuid,text,uuid),ledger.use_customer_challenge(bytea,text,uuid,uuid,timestamptz),
    ledger.prove_customer_account(bytea,text,uuid,timestamptz,bytea,uuid,bigint,bytea,integer,integer,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.prove_account_orders(bytea,bytea,timestamptz),ledger.customer_challenge_allowed(uuid,text,uuid),
    ledger.prove_customer_account(bytea,text,uuid,timestamptz,bytea,uuid,bigint,bytea,integer,integer,integer) TO nu54_p05_commerce;
