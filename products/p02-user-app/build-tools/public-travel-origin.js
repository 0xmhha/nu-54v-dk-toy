const path = require('node:path');

function approvedOrigin(raw) {
  if (!raw) return '';
  const candidate = raw.trim();
  const match = /^https:\/\/([a-z0-9.-]+|\[[0-9a-f:]+\])(:[0-9]{1,5})?\/?$/i.exec(candidate);
  if (!match) throw new Error('NU54_P09_PUBLIC_ORIGIN must be an approved HTTPS public origin');
  try {
    const url = new URL(candidate);
    if (!url.hostname || url.hostname.endsWith('.') || url.port === '0') throw new Error();
    return url.origin;
  } catch {
    throw new Error('NU54_P09_PUBLIC_ORIGIN must be an approved HTTPS public origin');
  }
}

function inlineTravelOrigin({types}, {origin}) {
  const target = path.resolve(__dirname, '../src/travel/publicDeployment.ts');
  return {
    visitor: {
      VariableDeclarator(declaration, state) {
        if (state.filename === target && declaration.node.id.name === 'approvedTravelOrigin') {
          declaration.node.init = types.stringLiteral(origin);
        }
      },
    },
  };
}

module.exports = {approvedOrigin, inlineTravelOrigin};
