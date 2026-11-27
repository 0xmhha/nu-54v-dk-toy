import {readdirSync, readFileSync, statSync} from 'fs';
import {join} from 'path';

// Users never see design page IDs (W07, T01, K10 ...) or product codes (P01, P03 ...); those are
// development terms. Comments may still use them.
const CODE = /\b(?:P0[0-9]|[WKTOLXD][0-9]{2})\b/;
const files = (dir: string): string[] => readdirSync(dir).flatMap(name => {
  const path = join(dir, name);
  return statSync(path).isDirectory() ? files(path) : /\.tsx?$/.test(name) ? [path] : [];
});

test('app text contains no design page IDs or product codes', () => {
  const found: string[] = [];
  for (const file of files(join(__dirname, '../src'))) {
    const code = readFileSync(file, 'utf8').replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/.*$/gm, '');
    const texts = [
      ...[...code.matchAll(/(['"`])((?:[^'"`\\]|\\.)*?)\1/g)].map(m => m[2]),
      ...[...code.matchAll(/>([^<>{}]+)</g)].map(m => m[1]),
    ];
    for (const text of texts) {
      // Korean or spaced English text is shown to people; bare identifiers are not.
      if (CODE.test(text) && (/[가-힣]/.test(text) || / [a-z]/.test(text))) found.push(`${file}: ${text.trim().slice(0, 80)}`);
    }
  }
  expect(found).toEqual([]);
});
