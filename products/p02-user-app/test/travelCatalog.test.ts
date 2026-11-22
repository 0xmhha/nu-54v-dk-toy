import {publishedOpening, readPublicCatalog} from '../src/travel/catalog.ts';
import {actionBranch, geographicPoint, publicSearchURL, type Place} from '../src/travel/catalog.ts';

const landmark: Place = {
  id: 'landmark', name: 'Public park', address: 'Public square',
  source: 'Synthetic curator', updatedAt: '2026-11-20T00:00:00Z',
  latitude: 37.53, longitude: 126.91,
};
const merchant: Place = {
  ...landmark, id: 'merchant', name: 'Venue', latitude: 37.57, longitude: 126.92,
  branchRef: '5dc8d4be-c24d-4553-ab79-9c30f630e477', capabilities: ['booking'],
};

test('manual search sends a region without location parameters or credentials', () => {
  expect(publicSearchURL('https://public.example', 'seoul')).toBe('https://public.example/v1/places?region=seoul');
  expect(() => publicSearchURL('https://me:secret@public.example', 'seoul')).toThrow();
  expect(() => publicSearchURL('https://public.example?latitude=37.53', 'seoul')).toThrow();
  expect(() => publicSearchURL('https://public.example', 'seoul&latitude=37.53')).toThrow();
  expect(() => publicSearchURL('http://public.example', 'seoul')).toThrow();
});

test('map keeps place IDs and booking action is specific to a published reference', () => {
  expect(geographicPoint(merchant)?.x).toBeGreaterThan(geographicPoint(landmark)!.x);
  expect(geographicPoint(merchant)?.y).toBeLessThan(geographicPoint(landmark)!.y);
  expect(geographicPoint({...landmark, latitude: undefined})).toBeUndefined();
  expect(actionBranch(landmark, 'booking')).toBeUndefined();
  expect(actionBranch({...merchant, capabilities: ['pickup']}, 'booking')).toBeUndefined();
  expect(actionBranch(merchant, 'booking')).toBe(merchant.branchRef);
});

test('published opening rules use the venue time zone, close boundary and the unknown distinction', () => {
  const venue = {
    id:'cafe', name:'Cafe', address:'Fixture', source:'Fixture source', updatedAt:'2026-07-04T00:00:00Z',
    opening:[{weekday:6, timezone:'America/New_York', opens:'10:00', closes:'11:00'}],
  };
  expect(publishedOpening(venue,new Date('2026-07-04T14:00:00Z'))).toBe('open');
  expect(publishedOpening(venue,new Date('2026-07-04T14:59:59Z'))).toBe('open');
  expect(publishedOpening(venue,new Date('2026-07-04T15:00:00Z'))).toBe('outside');
  expect(publishedOpening({...venue,opening:[]},new Date('2026-07-04T14:00:00Z'))).toBe('unknown');
});

test('catalog validation rejects foreign regions and versions with lost integer precision', () => {
  const place = {...landmark, region: 'seoul'};
  const current = {environment: 'public', version: 4, places:[place]};
  expect(readPublicCatalog(current, 'seoul')).toEqual(current);
  expect(readPublicCatalog({environment:'public', places:[place]}, 'seoul').version).toBeUndefined();
  for (const version of [0, -1, 1.5, '4', NaN, Infinity, 9007199254740992]) {
    expect(() => readPublicCatalog({...current, version}, 'seoul')).toThrow();
  }
  expect(() => readPublicCatalog({...current, places:[{...place, region:'busan'}]}, 'seoul')).toThrow();
  expect(() => readPublicCatalog({...current, places:[landmark]}, 'seoul')).toThrow();
});
