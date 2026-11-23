// The board process (../board/board_host.c) and the phone's radio to it.
import {spawn, type ChildProcess} from 'node:child_process';
import {randomBytes} from 'node:crypto';

type Answer = 'press' | 'refuse' | 'wrongpin' | 'ignore';

export class Board {
  private child: ChildProcess;
  private buffer = '';
  private waiting: {words: string[]; resolve: (line: string) => void}[] = [];
  /** What the owner does when the board asks for a button. */
  answer: Answer = 'press';
  /** How often the board asked, and which lamp events it showed (1 signed, 2 refused). */
  prompts = 0;
  lamps: number[] = [];
  erased = 0;
  device = '';
  account = '';
  generation = 0n;
  connected = false;
  /** The next signature never reaches the phone: the link ends once the board signed. */
  dropAfterSigning = false;
  private deaf = false;
  readonly sounds = {fragment: new Set<(chunk: string) => void>(), lost: new Set<() => void>()};

  constructor(binary: string) {
    this.child = spawn(binary, [], {stdio: ['pipe', 'pipe', 'inherit']});
    this.child.stdout!.setEncoding('utf8').on('data', (text: string) => {
      this.buffer += text;
      for (let end = this.buffer.indexOf('\n'); end >= 0; end = this.buffer.indexOf('\n')) {
        const line = this.buffer.slice(0, end);
        this.buffer = this.buffer.slice(end + 1);
        this.hear(line);
      }
    });
  }

  private hear(line: string) {
    const [word, ...rest] = line.split(' ');
    if (word === 'out') {
      if (!this.deaf) {
        const bytes = Buffer.from(rest[0], 'hex');
        for (let at = 0; at < bytes.length; at += 244) for (const run of this.sounds.fragment) run(bytes.subarray(at, at + 244).toString('base64'));
      }
    } else if (word === 'prompt') {
      this.prompts++;
      if (this.answer !== 'ignore') setTimeout(() => this.write(this.answer), 100);
    } else if (word === 'lamp') {
      this.lamps.push(Number(rest[0]));
      if (this.dropAfterSigning && rest[0] === '1') {
        this.dropAfterSigning = false;
        this.deaf = true;
        setTimeout(() => this.drop(), 50);
      }
    } else if (word === 'erased') this.erased++;
    else if (word === 'booted') [this.device, this.account, this.generation] = [rest[0], '0x' + rest[1], BigInt(rest[2])];
    const found = this.waiting.findIndex(item => item.words.includes(word));
    if (found >= 0) this.waiting.splice(found, 1)[0].resolve(line);
  }

  private write(line: string) { this.child.stdin!.write(line + '\n'); }

  wait(...words: string[]) {
    return new Promise<string>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('BOARD_SILENT ' + words.join())), 10_000);
      this.waiting.push({words, resolve: line => { clearTimeout(timer); resolve(line); }});
    });
  }
  async command(line: string, ...answers: string[]) {
    const heard = this.wait(...(answers.length ? answers : ['ok']));
    this.write(line);
    return heard;
  }
  /** Power cycle: the operator's grant is needed again, and `ahead` is how many seconds it took. */
  boot() { return this.command('boot', 'booted'); }
  grant(ahead = 2) { return this.command(`grant 3000 ${ahead}`); }

  /** The Bluetooth link ends without either side asking for it. */
  drop() {
    this.write('close');
    this.connected = false;
    for (const run of [...this.sounds.lost]) run();
  }
  async connect(epoch: Buffer, phone: string) {
    this.deaf = false;
    this.connected = true;
    return await this.command(`connect ${epoch.toString('hex')} ${phone}`, 'open', 'denied') === 'open';
  }
  data(base64: string) { if (this.connected) this.write('data ' + Buffer.from(base64, 'base64').toString('hex')); }
  disconnect() { if (this.connected) { this.connected = false; this.write('close'); } }
  stop() { this.child.stdin!.end(); this.child.kill(); }
}

export const epochOf = () => randomBytes(16);
