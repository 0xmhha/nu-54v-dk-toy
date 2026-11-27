export type Place = {
  id: string;
  name: string;
  region?: string;
  address: string;
  source: string;
  updatedAt: string;
  latitude?: number;
  longitude?: number;
  category?: 'restaurant' | 'attraction';
  opening?: {weekday: number; timezone: string; opens: string; closes: string}[];
  branchRef?: string;
  capabilities?: string[];
};

export type Catalog = {
  version?: number;
  environment: 'public' | 'synthetic';
  places: Place[];
};

export type Region = {code: string; name: string};
export type MapInfo = {minZoom: number; maxZoom: number; attribution: string};

const regionCode = /^[a-z0-9][a-z0-9_-]{0,47}$/;
const branchRef = /^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[0-9a-f]{12}$/;

export function publicOrigin(input: string) {
  // React Native's URL polyfill accepts several malformed authority strings.
  const origin = /^https:\/\/((?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?|\[[a-f0-9:]+\])(?::([0-9]{1,5}))?\/?$/i.exec(input.trim());
  if (!origin || (origin[2] && (Number(origin[2]) < 1 || Number(origin[2]) > 65535))) {
    throw new Error('Invalid public catalog origin');
  }
  const port = origin[2] && Number(origin[2]) !== 443 ? `:${Number(origin[2])}` : '';
  return `https://${origin[1].toLowerCase()}${port}`;
}

export function publicSearchURL(endpoint: string, region: string) {
  if (!regionCode.test(region)) throw new Error('Invalid public catalog region');
  return `${publicOrigin(endpoint)}/v1/places?region=${region}`;
}

export function actionBranch(place: Place, action: 'booking' | 'pickup') {
  if (place.branchRef && branchRef.test(place.branchRef) && place.capabilities?.includes(action)) {
    return place.branchRef;
  }
}

export function geographicPoint(place: Place) {
  const {latitude, longitude} = place;
  if (typeof latitude !== 'number' || !Number.isFinite(latitude) || Math.abs(latitude) > 90 ||
      typeof longitude !== 'number' || !Number.isFinite(longitude) || Math.abs(longitude) > 180) return;
  const radians = Math.max(-85.05112878, Math.min(85.05112878, latitude)) * Math.PI / 180;
  return {
    x: (longitude + 180) / 360,
    y: Math.max(0, Math.min(1, (1 - Math.asinh(Math.tan(radians)) / Math.PI) / 2)),
  };
}

export type PlaceFilters = {category: 'all' | 'restaurant' | 'attraction'; openNow: boolean; booking: boolean};
export const defaultPlaceFilters: PlaceFilters = {category: 'all', openNow: false, booking: false};
const weekdays = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const clockTime = /^(?:[01][0-9]|2[0-3]):[0-5][0-9]$/;

type VenueClock = {weekday: number; time: string};

function openingAt(place: Place, now: Date, localTimes: Map<string, VenueClock>): 'open' | 'outside' | 'unknown' {
  if (!place.opening?.length) return 'unknown';
  let incomplete = false;
  for (const interval of place.opening) {
    if (!Number.isInteger(interval.weekday) || interval.weekday < 0 || interval.weekday > 6 ||
        !interval.timezone || !clockTime.test(interval.opens) || !clockTime.test(interval.closes) || interval.opens >= interval.closes) {
      incomplete = true;
      continue;
    }
    let local = localTimes.get(interval.timezone);
    if (!local) {
      try {
        const parts = new Intl.DateTimeFormat('en-US', {timeZone: interval.timezone, weekday: 'short',
          hour: '2-digit', minute: '2-digit', hourCycle: 'h23'}).formatToParts(now);
        const get = (key: string) => parts.find(part => part.type === key)?.value ?? '';
        local = {weekday: weekdays.indexOf(get('weekday')), time: get('hour') + ':' + get('minute')};
        if (local.weekday < 0 || !clockTime.test(local.time)) { incomplete = true; continue; }
        localTimes.set(interval.timezone, local);
      } catch {
        incomplete = true;
        continue;
      }
    }
    if (local.weekday === interval.weekday && interval.opens <= local.time && local.time < interval.closes) return 'open';
  }
  return incomplete ? 'unknown' : 'outside';
}

export function publishedOpening(place: Place, now: Date) {
  return openingAt(place, now, new Map());
}

export function filterPlaces(places: Place[], query: string, filters: PlaceFilters, now: Date) {
  const term = query.trim().toLowerCase();
  const localTimes = new Map<string, VenueClock>();
  return places.filter(place =>
    (filters.category === 'all' || place.category === filters.category) &&
    (!filters.openNow || openingAt(place, now, localTimes) === 'open') &&
    (!filters.booking || !!actionBranch(place, 'booking')) &&
    [place.name, place.address].some(value => value.toLowerCase().includes(term)));
}

export function readPublicCatalog(value: unknown, region: string): Catalog {
  if (!value || typeof value !== 'object') throw new Error('Invalid public catalog');
  const catalog = value as Catalog;
  if (!['synthetic', 'public'].includes(catalog.environment) ||
      (catalog.version !== undefined && (!Number.isSafeInteger(catalog.version) || catalog.version < 1)) ||
      !Array.isArray(catalog.places) || catalog.places.length > 10000 ||
      !catalog.places.every(item => item && item.region === region && typeof item.id === 'string' && regionCode.test(item.id) &&
        typeof item.name === 'string' && typeof item.address === 'string' &&
        typeof item.source === 'string' && typeof item.updatedAt === 'string') ||
      new Set(catalog.places.map(item => item.id)).size !== catalog.places.length) {
    throw new Error('Invalid public catalog');
  }
  return catalog;
}
