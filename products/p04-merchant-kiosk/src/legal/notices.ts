// The legal notices of this build. scripts/legal_notices.py writes the file from the reviewed sources.
import generated from './notices.generated.json';

export type NoticeId = 'terms' | 'privacy' | 'location' | 'test-assets';
export type Notice = {
  id: NoticeId;
  language: 'ko' | 'en';
  title: string;
  edition: string;
  status: 'draft' | 'final';
  effective: string | null;
  sections: { heading: string; body: string }[];
};
export type License = { name: string; version: string; license: string; text: number };
type Notices = {
  version: number;
  publisher: { name: string | null; contact: string | null };
  documents: Notice[];
  licenses: License[];
  texts: string[];
};

export const notices = generated as unknown as Notices;
/** The document in the reader's language, or in Korean when that one does not exist. */
export const notice = (id: NoticeId, language: 'ko' | 'en', from: Notice[] = notices.documents) =>
  from.find(item => item.id === id && item.language === language) ??
  from.find(item => item.id === id && item.language === 'ko');
export const licenseText = (license: License) => notices.texts[license.text] ?? '';
/** The version people see; a test keeps it equal to the Android build's. */
export const appVersion = '1.0';
