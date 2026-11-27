/* BIP 380 descriptor checksum, https://bips.dev/380/ (BSD-2-Clause). */
const alphabet = "0123456789()[],'/*abcdefgh@:$%{}IJKLMNOPQRSTUVWXYZ&+-.;<=>?!^_|~ijklmnopqrstuvwxyzABCDEFGH`#\"\\ ";
const checksum = 'qpzry9x8gf2tvdw0s3jn54khce6mua7l';
const generators = [0xf5dee51989n, 0xa9fdca3312n, 0x1bab10e32dn, 0x3706b1677an, 0x644d626ffdn];

export function checksumDescriptor(descriptor: string): string {
  if (descriptor.includes('#')) throw new Error('BTC_DESCRIPTOR');
  let state = 1n;
  const update = (symbol: number) => {
    const high = state / (2n ** 35n);
    state = ((state % (2n ** 35n)) * 32n) ^ BigInt(symbol);
    for (const [bit, generator] of generators.entries()) {
      if ((high / (2n ** BigInt(bit))) % 2n) state ^= generator;
    }
  };
  let group = 0, digits = 0;
  for (const character of descriptor) {
    const index = alphabet.indexOf(character);
    if (index < 0) throw new Error('BTC_DESCRIPTOR');
    update(index % 32);
    group = 3 * group + Math.floor(index / 32);
    if (++digits === 3) { update(group); group = 0; digits = 0; }
  }
  if (digits) update(group);
  for (let i = 0; i < 8; i++) update(0);
  state ^= 1n;
  let result = '';
  for (let i = 7; i >= 0; i--) result += checksum[Number(state / (32n ** BigInt(i)) % 32n)];
  return descriptor + '#' + result;
}
