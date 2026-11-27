import { readFileSync } from "node:fs";
import test from "node:test";
import assert from "node:assert/strict";

import {
  bindServiceRequestV2, validateServiceReplyV2,
  type ServiceReplyV2, type ServiceRequestV2, type ServiceScopeV2,
} from "../src/index.ts";

const fixtures: { id: string; at: number; request: ServiceRequestV2; scope: ServiceScopeV2;
  intentDigest: string; requestDigest: string; reply?: ServiceReplyV2 }[] =
  JSON.parse(readFileSync(new URL("../../v2/service-vectors.json", import.meta.url), "utf8"));

for (const value of fixtures) {
  test(`v2 service binding ${value.id}`, () => {
    const bound = bindServiceRequestV2(value.request, value.scope, value.at);
    assert.equal(bound.requestDigest, value.requestDigest);
    assert.equal(bound.intentDigest, value.intentDigest);
    assert.throws(() => bindServiceRequestV2({ ...value.request, expiresAt: value.at - 1 }, value.scope, value.at), /EXPIRED/);
    assert.throws(() => bindServiceRequestV2({ ...value.request, objectRef: value.request.requestId }, value.scope, value.at),
      /NOT_AUTHORIZED/);
    if (value.reply?.evidence) {
      validateServiceReplyV2(value.request, bound, value.reply, value.at);
      assert.throws(() => validateServiceReplyV2(value.request, bound,
        { ...value.reply!, evidence: { ...value.reply!.evidence!, generationRef: value.request.objectRef } }, value.at),
      /MISMATCHED_EVIDENCE/);
    }
  });
}
