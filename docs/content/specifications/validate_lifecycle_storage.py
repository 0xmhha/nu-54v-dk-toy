"""Validate a storage design inventory only; never applies SQL or simulates hardware."""
import hashlib,json,re
from pathlib import Path
P=Path(__file__).resolve().parent

def read(n):return json.loads((P/n).read_text())
def need(ok,msg):
    if not ok:raise ValueError(msg)

d=read('lifecycle-storage-design.json')
need(d['implementation']=='deferred_by_user' and not any(d[k]for k in ['canonicalMerged','runtimeVerified','sqlApplied']),'design status')
names=[t['name']for t in d['tables']];need(len(names)==len(set(names))==14,'table uniqueness')
for t in d['tables']:
 fields=[x.split(':',1)[0]for x in t['fields']]
 need(len(fields)==len(set(fields)),t['name']+' duplicate field')
 need(set(t['primaryKey'])<=set(fields),t['name']+' primary key')
 need(t['databaseConstraints'] and t['transactionRules'] and t['status']=='proposed_not_created','table coverage')
existing=set()
for name,h in d['referenceSqlHashes'].items():
 p=P/'database'/name;need(hashlib.sha256(p.read_bytes()).hexdigest()==h,'SQL drift '+name)
 existing.update(re.findall(r'CREATE TABLE\s+(\w+)',p.read_text()))
need(len(existing-{'schema_migrations'})==d['baseline']['tables']==61 and len(d['referenceSqlHashes'])==7,'SQL baseline')
for name,h in d['sourceHashes'].items():need(hashlib.sha256((P/name).read_bytes()).hexdigest()==h,'source drift '+name)
prior={x['name']for x in read('approval-physical-storage-design.json')['tables']}
routes=set()
for f in ['rental-admission-cancel.json','return-route-contracts.json']:
 x=read(f);routes.update(y['id']for y in x['operations']+x['bleCommands'])
units={u['id']:u for u in d['atomicUnits']};need(len(units)==12,'atomic units')
for u in units.values():
 need(set(u['routeRefs'])<=routes,'unknown route')
 need(set(u['reads']+u['candidateWrites'])<=set(names),'unknown proposed table')
 need(set(u['existingWrites'])<=existing,'unknown existing table')
 need(set(u['commonWrites'])<=existing|prior,'unknown shared table')
 need('approval_authorization_gates'in u['commonWrites'],'shadow authority gate')
 need(not u['externalEffectsInTransaction'] and not u['runtimeVerified'],'runtime claim')
for effect in d['existingTableEffects']:need(set(effect['writes'])<=set(units[effect['unit']]['existingWrites']),'missing existing write')
need({'rentals','device_bindings','wallet_bindings'}<=set(units['LST-T03']['existingWrites']),'return/revoke outside commit')
need({'lifecycle_objects','lifecycle_permits','lifecycle_relay_authorities'}<=set(units['LST-T11']['candidateWrites']),'relay activation/link outside commit')
need('lifecycle_evidence' in units['LST-T06']['candidateWrites'],'evidence apply outside commit')
need({'rentals','device_bindings'}<=set(units['LST-T05']['existingWrites']),'reservation outside commit')
need([x['rank']for x in d['lockOrder']]==[0,10,15,20,30,40,50,60],'lock order')
need(set(names)<=set(d['lockOrder'][2]['targets']),'unranked lifecycle table')
done=set()
for m in d['migrations']:
 need(set(m['dependsOn'])<=done and not m['executed'],'migration dependency/execution');done.add(m['id'])
need(len(done)==8 and len(d['firmwareStores'])==4,'recovery/phase inventory')
need(len(d['recoveryCases'])==26 and all(x['status']=='not_run'for x in d['recoveryCases']),'runtime cases')
need({x['id']for x in d['schemaDeltas']}=={'LST-D01','LST-D02','LST-D03','LST-D04'},'known compatibility gaps')
q=next(x for x in read('return-recovery-contract.json')['openDecisions']if x['id']=='RR-DEC-01');need(q['selection']is None and q['status']=='awaiting_user_preference','policy drift')
print(json.dumps({'scope':'storage_design_inventory_only','proposedTables':14,'atomicUnits':12,'firmwareStores':4,'migrationPhasesNotRun':8,'runtimeCasesNotRun':26,'unchangedSqlTables':61,'unchangedSqlFiles':7,'pinnedDesignInputs':len(d['sourceHashes']),'canonicalMerged':False},ensure_ascii=False))
