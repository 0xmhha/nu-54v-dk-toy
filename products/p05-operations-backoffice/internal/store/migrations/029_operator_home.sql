CREATE TABLE ledger.operator_branch_label (
    branch_id uuid PRIMARY KEY REFERENCES ledger.branch,
    label text NOT NULL CHECK(length(btrim(label)) BETWEEN 1 AND 120)
);
REVOKE ALL ON ledger.operator_branch_label FROM PUBLIC;

CREATE FUNCTION ledger.operator_scopes(tenant uuid,actor uuid,role_ text)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce(jsonb_agg(jsonb_build_object('id',b.id,'label',coalesce(n.label,'등록 이름 없음 · '||right(b.id::text,8)),'timezone',b.timezone,
        'capabilities',g.capabilities) ORDER BY n.label,b.id),'[]'::jsonb)
    FROM ledger.branch b LEFT JOIN ledger.operator_branch_label n ON n.branch_id=b.id
    JOIN LATERAL (
       SELECT array_agg(DISTINCT permission ORDER BY permission) AS capabilities
       FROM (
         SELECT 'catalog'::text AS permission FROM ledger.staff_grant s WHERE role_='operator' AND s.branch_id=b.id
           AND s.subject_ref=actor AND s.role_code='operator' AND s.valid_from<=clock_timestamp() AND s.revoked_at IS NULL
         UNION ALL
         SELECT f.capability FROM ledger.finance_grant f WHERE f.branch_id=b.id AND f.actor_id=actor AND
           f.valid_until>clock_timestamp() AND
           (role_='finance' AND f.capability IN ('refund_request','refund_approve','day_close','report_read','report_export','reconcile_read','reconcile_retry')
            OR role_='operator' AND f.capability IN ('experience_read','experience_cancel','payout_propose','payout_approve','payout_notice',
                   'rental_manage','firmware_manage','firmware_station','access_manage','audit_read','legacy_ledger','legacy_cashout','legacy_reconcile'))
       ) authorized
    ) g ON cardinality(g.capabilities)>0
    WHERE b.tenant_ref=tenant;
$$;

CREATE FUNCTION ledger.operator_catalog_home(tenant uuid,branch uuid,actor uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT ledger.operator_catalog_with_inventory(tenant,branch,actor) || jsonb_build_object('timezone',b.timezone)
    FROM ledger.branch b WHERE b.id=branch
$$;
REVOKE ALL ON FUNCTION ledger.operator_scopes(uuid,uuid,text),ledger.operator_catalog_home(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_scopes(uuid,uuid,text) TO nu54_p05_commerce,nu54_p05_finance;
GRANT EXECUTE ON FUNCTION ledger.operator_catalog_home(uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE FUNCTION ledger.name_operator_branch(tenant uuid,branch uuid,actor uuid,label_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    IF label_<>btrim(label_) OR length(label_) NOT BETWEEN 1 AND 120 OR label_ ~ '[[:cntrl:]]' THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid operating label';
    END IF;
    INSERT INTO ledger.operator_branch_label VALUES(branch,label_) ON CONFLICT(branch_id) DO UPDATE SET label=EXCLUDED.label;
    RETURN ledger.operator_scopes(tenant,actor,'operator');
END $$;
REVOKE ALL ON FUNCTION ledger.name_operator_branch(uuid,uuid,uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.name_operator_branch(uuid,uuid,uuid,text) TO nu54_p05_commerce;
