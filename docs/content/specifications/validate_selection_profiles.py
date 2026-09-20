"""Finite design decision-table checks, not an authorization implementation.
Synthetic booleans stand for verified evidence; this does not verify that evidence.
"""
import copy,hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent
profile=json.loads((P/'selection-profile-contract.json').read_text())
gates=json.loads((P/'selection-action-gates.json').read_text())
A={a['id']:a for a in gates['actions']}
FIELDS={f['id'] for p in profile['profiles'] for f in p['fields']}

def field_slice(action,variant):
    if action.get('branches'):
        br=next((b for b in action['branches'] if b['when']==variant),None)
        return set(br['requiredFieldRefs']) if br else None
    return set(action['requiredFieldRefs']) if variant is None else None


def assess(action_id,context,variant=None):
    a=A.get(action_id)
    if not a:return 'hold:UNSUPPORTED_PROFILE'
    required=field_slice(a,variant)
    if required is None:return 'hold:UNSUPPORTED_PROFILE'
    # Valid authority may be an explicitly permitted scoped result-recovery proof.
    if not context['authorized'] or not context['contextCurrent']:return 'hold:FORBIDDEN'
    if not context['trustedCapability']:return 'hold:UNSUPPORTED_PROFILE'
    if not context['securityAllowsThisMode']:return 'hold:AUTHORITY_HELD'
    k=a['class']
    if k=='read_observation':
        return 'read_only' if context['readProjectionAllowed'] else 'hold:PRIVACY_DENIED'
    if k=='safety_restriction':
        return 'restriction_only' if context['restrictionIsNarrow'] else 'hold:FORBIDDEN'
    if k=='scoped_cleanup':
        if not context['scopedPlanCurrent']:return 'hold:POLICY_UNSELECTED'
        if not context['planTargetsCurrent']:return 'hold:REVISION_CONFLICT'
        return 'cleanup_only' if context['domainGuardsPass'] else 'hold:AUTHORITY_HELD'
    if k=='continue_existing':
        if not context['originalProfileKnown'] or not context['originalBindingMatches']:return 'hold:UNSUPPORTED_PROFILE'
        if not required <= set(context['originalFields']):return 'hold:POLICY_UNSELECTED'
        if not context['continuationAllowed'] or context['wouldCreateNewEffect']:return 'hold:AUTHORITY_HELD'
    else:
        if not required <= set(context['selectedFields']):return 'hold:POLICY_UNSELECTED'
        if not context['profileActiveForAction'] or not context['runtimeEvidenceForAction']:return 'hold:UNSUPPORTED_PROFILE'
        if not context['activationHeadCurrent']:return 'hold:REVISION_CONFLICT'
    if context['originalExposureUnknown']:return 'hold:EXPOSURE_UNKNOWN'
    if not context['domainGuardsPass']:return 'hold:AUTHORITY_HELD'
    if action_id=='GA-41':
        if variant=='new_travel_hold_without_recovery':return 'hold:AUTHORITY_HELD'
        if variant=='new_travel_backup_reset' and (context['returnPolicy']!='add_user_encrypted_recovery_backup' or not context['independentRecoveryVerified']):return 'hold:AUTHORITY_HELD'
    if action_id=='GA-40' and not context['inventoryOrCleanupProven']:return 'hold:AUTHORITY_HELD'
    if action_id=='GA-44' and variant=='apply_candidate' and not context['candidateBaseCurrent']:return 'hold:REVISION_CONFLICT'
    return 'eligible_for_domain_checks'

# A deliberately synthetic baseline, never a real selected profile.
base={k:True for k in ['authorized','contextCurrent','trustedCapability','securityAllowsThisMode','readProjectionAllowed',
 'restrictionIsNarrow','scopedPlanCurrent','planTargetsCurrent','domainGuardsPass','originalProfileKnown','originalBindingMatches',
 'continuationAllowed','profileActiveForAction','runtimeEvidenceForAction','activationHeadCurrent','independentRecoveryVerified',
 'inventoryOrCleanupProven','candidateBaseCurrent']}
base.update(selectedFields=sorted(FIELDS),originalFields=sorted(FIELDS),wouldCreateNewEffect=False,originalExposureUnknown=False,
            returnPolicy='add_user_encrypted_recovery_backup')
cases=[]
def case(name,action,expected,variant=None,**changes):
    c=copy.deepcopy(base);c.update(changes);actual=assess(action,c,variant)
    assert actual==expected,(name,actual,expected)
    # Only changed values and an explicit baseline label are emitted to keep the result readable.
    cases.append(dict(id=name,actionId=action,variant=variant,syntheticBaseline='all_required_evidence_true_fixture_not_runtime',
                      changes=changes,expected=expected,observed=actual))
case('unknown_action','GA-unknown','hold:UNSUPPORTED_PROFILE')
case('unknown_branch','GA-44','hold:UNSUPPORTED_PROFILE','unknown')
for action in ['GA-02','GA-03','GA-27','GA-07','GA-19','GA-41']:
    v='imported_wallet_copy_reset' if action=='GA-41' else None
    case(action+'_no_authority',action,'hold:FORBIDDEN',v,authorized=False)
    case(action+'_stale_context',action,'hold:FORBIDDEN',v,contextCurrent=False)
    case(action+'_untrusted_reader_or_writer',action,'hold:UNSUPPORTED_PROFILE',v,trustedCapability=False)
    case(action+'_current_security_denial',action,'hold:AUTHORITY_HELD',v,securityAllowsThisMode=False)
case('read_without_new_policy','GA-20','read_only',selectedFields=[],profileActiveForAction=False)
case('read_original_unknown_not_new_payment','GA-20','read_only',originalExposureUnknown=True)
case('read_denied_by_current_projection','GA-17','hold:PRIVACY_DENIED',readProjectionAllowed=False)
case('refresh_scoped_recovery_proof','GA-53','read_only',selectedFields=[],profileActiveForAction=False)
case('logout_without_new_policy','GA-03','restriction_only',selectedFields=[],profileActiveForAction=False)
case('withdraw_without_retention_choice','GA-47','restriction_only',selectedFields=[])
case('stop_audio_even_if_new_use_denied','GA-15','restriction_only',selectedFields=[],readProjectionAllowed=False)
case('logout_cannot_expand_to_key_erase','GA-03','hold:FORBIDDEN',restrictionIsNarrow=False)
case('terminal_scoped_cleanup_without_new_policy','GA-27','cleanup_only',selectedFields=[])
case('delete_existing_approved_scope','GA-51','cleanup_only',selectedFields=[])
case('delete_no_plan','GA-51','hold:POLICY_UNSELECTED',scopedPlanCurrent=False)
case('delete_late_new_target','GA-51','hold:REVISION_CONFLICT',planTargetsCurrent=False)
case('delete_retention_hold','GA-51','hold:AUTHORITY_HELD',domainGuardsPass=False)
case('new_payment_no_choice','GA-19','hold:POLICY_UNSELECTED',selectedFields=[])
case('proposal_values_not_active','GA-19','hold:UNSUPPORTED_PROFILE',profileActiveForAction=False)
case('contract_validated_not_runtime','GA-19','hold:UNSUPPORTED_PROFILE',runtimeEvidenceForAction=False)
case('profile_switch_before_commit','GA-19','hold:REVISION_CONFLICT',activationHeadCurrent=False)
case('unknown_original_payout','GA-19','hold:EXPOSURE_UNKNOWN',originalExposureUnknown=True)
case('new_payment_domain_still_checked','GA-19','hold:AUTHORITY_HELD',domainGuardsPass=False)
case('positive_fixture_is_only_eligibility','GA-19','eligible_for_domain_checks')
case('resume_original_retired_profile','GA-07','eligible_for_domain_checks',selectedFields=[],profileActiveForAction=False)
case('resume_cannot_guess_latest_profile','GA-07','hold:UNSUPPORTED_PROFILE',originalProfileKnown=False)
case('resume_wrong_original_digest','GA-07','hold:UNSUPPORTED_PROFILE',originalBindingMatches=False)
case('resume_incomplete_original_fields','GA-07','hold:POLICY_UNSELECTED',originalFields=[])
case('resume_requires_current_permission','GA-07','hold:AUTHORITY_HELD',continuationAllowed=False)
case('resume_cannot_start_new_dkg','GA-07','hold:AUTHORITY_HELD',wouldCreateNewEffect=True)
case('resume_unknown_commit','GA-07','hold:EXPOSURE_UNKNOWN',originalExposureUnknown=True)
case('refund_not_a_read','GA-21','hold:POLICY_UNSELECTED',selectedFields=[])
case('perp_close_not_a_read','GA-33','hold:POLICY_UNSELECTED',selectedFields=[])
case('export_not_a_read','GA-18','hold:POLICY_UNSELECTED',selectedFields=[])
case('consent_grant_not_withdrawal','GA-50','hold:POLICY_UNSELECTED',selectedFields=[])
case('reset_without_choice','GA-41','hold:POLICY_UNSELECTED','new_travel_backup_reset',selectedFields=[])
case('backup_choice_without_recovery','GA-41','hold:AUTHORITY_HELD','new_travel_backup_reset',independentRecoveryVerified=False)
case('backup_path_wrong_policy','GA-41','hold:AUTHORITY_HELD','new_travel_backup_reset',returnPolicy='hold_reset_without_recovery')
case('hold_policy_never_erases','GA-41','hold:AUTHORITY_HELD','new_travel_hold_without_recovery',returnPolicy='hold_reset_without_recovery')
case('import_does_not_need_travel_backup','GA-41','eligible_for_domain_checks','imported_wallet_copy_reset',selectedFields=[f for f in FIELDS if not f.startswith('PF-RR-') and f!='PF-D03-04'])
case('new_rental_independent_of_refund_rules','GA-40','eligible_for_domain_checks','new_rental',selectedFields=sorted(field_slice(A['GA-40'],'new_rental')))
case('readmit_requires_cleanup_evidence','GA-40','hold:AUTHORITY_HELD','complete_return_or_readmit',inventoryOrCleanupProven=False)
case('manual_edit_without_ai_provider','GA-44','eligible_for_domain_checks','manual_edit',selectedFields=sorted(field_slice(A['GA-44'],'manual_edit')))
case('generation_needs_ai_provider','GA-44','hold:POLICY_UNSELECTED','generate_candidate',selectedFields=[f for f in FIELDS if f!='PF-D18-02'])
case('late_candidate_cannot_overwrite_edit','GA-44','hold:REVISION_CONFLICT','apply_candidate',candidateBaseCurrent=False)
case('server_verify_without_device_signer','GA-35','eligible_for_domain_checks','verify_presentation',selectedFields=sorted(field_slice(A['GA-35'],'verify_presentation')))
case('device_signature_needs_key_profile','GA-35','hold:POLICY_UNSELECTED','issue_or_present_with_device',selectedFields=sorted(field_slice(A['GA-35'],'verify_presentation')))
case('alert_independent_of_ranging','GA-08','eligible_for_domain_checks',selectedFields=[f for f in FIELDS if f!='PF-D07-04'])
case('ranging_requires_measurement_profile','GA-09','hold:POLICY_UNSELECTED',selectedFields=[f for f in FIELDS if f!='PF-D07-04'])
case('defi_independent_of_perp_selection','GA-32','eligible_for_domain_checks',selectedFields=[f for f in FIELDS if not f.startswith('PF-D13-')])
case('scope_filtered_read_requires_reader','GA-02','hold:UNSUPPORTED_PROFILE',trustedCapability=False,selectedFields=[])
report=dict(status='passed',scope='finite_design_gate_examples_only',profiles=20,fields=len(FIELDS),crossConstraints=16,
            representativeActions=len(A),casesPassed=len(cases),cases=cases,selectedValues=0,runtimeVerified=False,
            productImplementationPerformed=False,canonicalMerged=False,
            limitations=['The evaluator is a documentation truth table, not runtime middleware.',
                         'Boolean evidence stands for independently verified authority/capability, not a client input.',
                         'Cross-constraint prose and all action subtypes are not formally proven by these cases.',
                         'No crypto, hardware, provider, persistence race or on-chain execution was performed.'],
            sourceHashes={f'content/specifications/{name}':hashlib.sha256((P/name).read_bytes()).hexdigest() for name in ['selection-profile-contract.json','selection-action-gates.json']})
(P/'selection-profile-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['cases','limitations','sourceHashes']},ensure_ascii=False))
