#!/usr/bin/env python3
"""Exercise P07 and P10 migrations via their Make targets on a disposable PG15 database."""

import os
import subprocess
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from uuid import uuid4

ROOT = Path(__file__).resolve().parents[3]
TEST_DSN = os.environ["P07_P10_MIGRATION_TEST_URL"]


def sql(statement, dsn=TEST_DSN):
    return subprocess.check_output(
        ["psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "--dbname", dsn, "-f", "-"],
        input=statement, text=True,
    ).strip()


def migrate(product, owner):
    subprocess.run(
        ["make", "-C", str(ROOT / "products" / product), "migrate"],
        env={**os.environ, f"{owner}_MIGRATION_DATABASE_URL": TEST_DSN}, check=True,
    )


def check_switch_race():
    stream, gen, first, second = [uuid4() for _ in range(4)]
    sql(f"""
        BEGIN;
        INSERT INTO chainview.stream VALUES ('{stream}', 'race', 'test', 'abi', '{stream}');
        INSERT INTO chainview.generation (id, stream_id, base_height, fence, state)
            VALUES ('{gen}', '{stream}', 0, 0, 'building');
        INSERT INTO chainview.block VALUES ('{gen}', 'block', 0, 'parent', now(), true);
        UPDATE chainview.generation SET state = 'ready', tip_height = 0, tip_hash = 'block',
            finalized_height = 0, finalized_hash = 'block' WHERE id = '{gen}';
        INSERT INTO chainview.switch_request
            (id, stream_id, target_id, expected_revision, operator_ref, compare_ref, requested_at)
        VALUES ('{first}', '{stream}', '{gen}', 0, gen_random_uuid(), gen_random_uuid(), now()),
               ('{second}', '{stream}', '{gen}', 0, gen_random_uuid(), gen_random_uuid(), now());
        COMMIT;
    """)

    def apply(request):
        return sql(f"""
            SET ROLE nu54_p07_switch;
            DO $test$
            BEGIN
                PERFORM chainview.apply_switch('{request}');
            EXCEPTION WHEN object_not_in_prerequisite_state THEN NULL;
            END
            $test$;
            SELECT applied_at IS NOT NULL FROM chainview.switch_request WHERE id = '{request}';
        """)

    with ThreadPoolExecutor(max_workers=2) as executor:
        results = list(executor.map(apply, (first, second)))
    assert sorted(results) == ["f", "t"], results
    assert sql(f"SELECT revision FROM chainview.read_target WHERE stream_id = '{stream}';") == "1"


def main():
    assert sql("SELECT current_setting('server_version_num')::int / 10000;") == "15"
    receipts = ROOT / "products/p07-indexer/migrations/001_receipts.sql"
    assert sql("SELECT to_regclass('public.receipts') IS NULL;") == "t"
    legacy_data = """
        INSERT INTO public.receipts VALUES ('tx', 0, 'merchant', 'order', 'device', 30, 1, 42, 1800000000);
        INSERT INTO public.cursor VALUES (1, 42);
    """
    sql(receipts.read_text() + legacy_data)
    before = sql("TABLE public.receipts; TABLE public.cursor;")
    for _ in range(2):
        migrate("p07-indexer", "P07")
        migrate("p10-platform", "P10")
    assert sql("TABLE public.receipts; TABLE public.cursor;") == before
    assert sql("SELECT count(*) FROM chainview.generation;") == "0"
    sql(Path(__file__).with_name("test_owner_migrations.sql").read_text())
    check_switch_race()


if __name__ == "__main__":
    main()
