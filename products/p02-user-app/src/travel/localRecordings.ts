import Native from '../specs/NativeCommerceHttp.ts';
import {erasureInstallationProof, opaqueTravelID} from './privateClient.ts';

export type LocalRecording = {id: string; capturedAt?: string};
export const deviceJournal = (id: string) => `travel-local-${id}`;
const shelf = deviceJournal('00000000-0000-4000-8000-000000000001');
export const maxLocalRecordings = 20;

export async function readLocalRecordings(): Promise<LocalRecording[]> {
  const raw = await Native?.loadJournal(shelf);
  if (!raw) return [];
  const entries: unknown = JSON.parse(raw);
  if (!Array.isArray(entries) || entries.length > maxLocalRecordings || entries.some(
    item => !item || typeof item.id !== 'string' || !opaqueTravelID.test(item.id) ||
      (item.capturedAt !== undefined && (typeof item.capturedAt !== 'string' || !Number.isFinite(Date.parse(item.capturedAt)))),
  )) throw new Error('Invalid local recording index');
  return entries;
}

export async function writeLocalRecordings(entries: LocalRecording[]) {
  if (!Native || entries.length > maxLocalRecordings) throw new Error('Local recording index unavailable');
  await Native.saveJournal(shelf, JSON.stringify(entries));
}

export type ErasureIntent = {object: string; key: string; receipt?: string; proof?: string};
const deletionJournal = deviceJournal('00000000-0000-4000-8000-000000000002');

export async function readErasureIntent(): Promise<ErasureIntent | undefined> {
  const raw = await Native?.loadJournal(deletionJournal);
  if (!raw) return undefined;
  const intent = JSON.parse(raw);
  if (!intent || !opaqueTravelID.test(intent.object) || !opaqueTravelID.test(intent.key) ||
    (intent.receipt !== undefined && !opaqueTravelID.test(intent.receipt)) ||
    (intent.proof !== undefined && !erasureInstallationProof.test(intent.proof))) {
    throw new Error('Invalid local erasure intent');
  }
  return intent;
}

export async function writeErasureIntent(intent?: ErasureIntent) {
  if (!Native) throw new Error('Local erasure journal unavailable');
  await Native.saveJournal(deletionJournal, intent ? JSON.stringify(intent) : '');
}
