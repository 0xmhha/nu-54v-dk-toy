-- Gates of the customer entrance that came out of its second review. No table or column is added; the challenge
-- table takes one more purpose.

-- A store takes customers only while its page is published (migration 123). Migration 122 gave a challenge to
-- anyone who knew a branch's reference, so a store that never published its page, or took it back, could still be
-- ordered from through a session. Published is now the gate of a new customer as well:
--   checkout  the store's page is published; any other store answers like a branch that does not exist
--   order, pickup  the same, or the order named is one this store already took payment for (being checked or paid):
--     a customer who paid follows, answers and collects that order even after the store took its page back
-- The proof checks the store again for a checkout (ledger.prove_customer_account, migration 125), and a checkout
-- session ends when the store takes its page back (ledger.customer_session_live below).
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

CREATE OR REPLACE FUNCTION ledger.issue_customer_challenge(branch_ uuid,purpose_ text,sale_ uuid,hash_ bytea,seconds_ integer)
RETURNS timestamptz LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $challenge$
DECLARE expiry timestamptz;
BEGIN
    IF seconds_ IS NULL OR seconds_ NOT BETWEEN 1 AND 300 OR purpose_ IS NULL THEN
        RAISE EXCEPTION 'invalid challenge' USING ERRCODE='23514';
    END IF;
    IF purpose_='account' THEN
        IF branch_ IS NOT NULL OR sale_ IS NOT NULL THEN
            RAISE EXCEPTION 'invalid challenge' USING ERRCODE='23514';
        END IF;
    ELSIF branch_ IS NULL OR NOT (ledger.store_page_published(branch_)
        OR (purpose_ IN ('order','pickup') AND EXISTS (SELECT FROM ledger.sale s
            WHERE s.id=sale_ AND s.branch_id=branch_ AND s.state IN ('checking','paid')))) THEN
        -- One answer for a branch that does not exist and for a store that is not public.
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

-- Accepts one signed `account` challenge whose account opsd has already checked, and answers the number of
-- unfinished orders of that account and nothing else: no order, no store, no amount. NULL when the challenge is not
-- an unused, unexpired `account` challenge; opsd then treats the proof as one of another purpose
-- (ledger.prove_customer_account), which refuses what it does not know.
CREATE FUNCTION ledger.prove_account_orders(hash_ bytea,payer_ bytea) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $orders$
BEGIN
    IF octet_length(payer_) IS DISTINCT FROM 20 THEN
        RAISE EXCEPTION 'invalid proof' USING ERRCODE='23514';
    END IF;
    UPDATE ledger.customer_challenge x SET used_at=clock_timestamp()
        WHERE x.challenge_hash=hash_ AND x.purpose='account' AND x.used_at IS NULL AND x.expires_at>clock_timestamp();
    IF NOT FOUND THEN RETURN NULL; END IF;
    RETURN ledger.payer_open_orders(payer_);
END
$orders$;

-- A store's registered name is shown to customers on its page, so it is one visible line like the page's own text
-- (ledger.store_text_plain, migration 123). The rule of migration 029 refused only control characters. Names saved
-- before this are not checked again; a store that saves its name anew is.
ALTER TABLE ledger.operator_branch_label ADD CONSTRAINT operator_branch_label_plain
    CHECK (ledger.store_text_plain(label)) NOT VALID;

REVOKE ALL ON FUNCTION ledger.payer_open_orders(bytea),ledger.prove_account_orders(bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.prove_account_orders(bytea,bytea) TO nu54_p05_commerce;
