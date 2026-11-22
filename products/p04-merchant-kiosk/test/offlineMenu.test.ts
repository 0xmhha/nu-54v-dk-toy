import type { CommercePrice } from '@nu54/protocol';
import { encodeCachedMenu, readCachedMenu } from '../src/kiosk/offlineMenu.tsx';

const scope='c'.repeat(64);
const branch='5dc8d4be-c24d-4553-ab79-9c30f630e477';
const checkedAt=1_700_000_000;
const rows:CommercePrice[]=Array.from({length:150},(_,index)=>({
  sku:`item-${index}`, optionKey:'plain', label:'커피', asset:'0x'+'2'.repeat(40),
  unitPrice:'3500000', version:2, available:index%3,
  expiresAt:new Date((checkedAt+100)*1000).toISOString(),
}));
const shop={branch,branchLabel:'여울 커피 성수점'};
const value={scope,shop,checkedAt,modeRevision:2,rows};

test('K01 can restore all 150 rows after expiry as the last verified snapshot',()=>{
  const saved=encodeCachedMenu(value);
  expect(saved).toBeDefined();
  expect(readCachedMenu(saved!,scope,branch,checkedAt+110)?.rows).toEqual(rows);
  expect(encodeCachedMenu({...value,rows:[...rows,{...rows[0],sku:'extra'}]})).toBeUndefined();
});

test('K01 rejects oversize, wrong branch and mixed or duplicate publication rows',()=>{
  const saved=encodeCachedMenu(value)!;
  expect(readCachedMenu(saved,scope,branch.replace('477','478'),checkedAt)).toBeUndefined();
  expect(readCachedMenu(saved+' '.repeat(128000),scope,branch,checkedAt)).toBeUndefined();
  const other={...rows[0],version:3};
  expect(encodeCachedMenu({...value,rows:[other,...rows.slice(1)]})).toBeUndefined();
  expect(encodeCachedMenu({...value,rows:[rows[1],...rows.slice(1)]})).toBeUndefined();
  expect(encodeCachedMenu({...value,rows:[{...rows[0],label:'name\u202E'},...rows.slice(1)]})).toBeUndefined();
});

test('K01 caches agreed public category metadata and rejects conflicting definitions', () => {
  const category = {key:'warm', sortOrder:10, labels:{ko:'음료', en:'Drinks'}};
  const categorized = [{...rows[0], category}, ...rows.slice(1, 51), {...rows[51], category}];
  const saved = encodeCachedMenu({...value, rows: categorized});
  expect(saved).toBeDefined();
  expect(readCachedMenu(saved!, scope, branch, checkedAt)?.rows).toEqual(categorized);
  categorized[51] = {...categorized[51], category:{...category, labels:{ko:'음료', en:'Other'}}};
  expect(encodeCachedMenu({...value, rows: categorized})).toBeUndefined();
});
