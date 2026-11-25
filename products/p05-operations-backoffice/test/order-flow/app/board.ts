// The board process (../board/board_host.c) and the phone's radio to it.
import {spawn, type ChildProcess} from 'node:child_process';
import {randomBytes} from 'node:crypto';
import {SendStandIn} from './boardSend';

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
  /** Answers what the firmware does not know yet. */
  readonly sends: SendStandIn;
  readonly sounds = {fragment: new Set<(chunk: string) => void>(), lost: new Set<() => void>()};

  constructor(binary: string, tokenName: string) {
    this.sends = new SendStandIn(tokenName);
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
      this.speak(this.sends.board(Buffer.from(rest[0], 'hex')));
    } else if (word === 'prompt') this.prompted(); else if (word === 'lamp') {
      this.lamps.push(Number(rest[0]));
      if (this.dropAfterSigning && rest[0] === '1') {
        this.dropAfterSigning = false;
        this.deaf = true;
        setTimeout(() => this.drop(), 50);
      }
    } else if (word === 'erased') { this.erased++; this.sends.forget(); }
    else if (word === 'booted') { this.sends.reset(); [this.device, this.account, this.generation] = [rest[0], '0x' + rest[1], BigInt(rest[2])]; }
    const found = this.waiting.findIndex(item => item.words.includes(word));
    if (found >= 0) this.waiting.splice(found, 1)[0].resolve(line);
  }

  private speak(bytes: Uint8Array) {
    if (this.deaf) return;
    for (let at = 0; at < bytes.length; at += 244) for (const run of this.sounds.fragment) run(Buffer.from(bytes.subarray(at, at + 244)).toString('base64'));
  }
  private prompted() {
    this.prompts++;
    if (this.answer !== 'ignore') setTimeout(() => { this.press(this.answer as Exclude<Answer, 'ignore'>).catch(() => {}); }, 100);
  }
  /** The owner's press goes to whichever of the two asked for it. */
  private async press(how: Exclude<Answer, 'ignore'>) {
    if (!this.sends.awaits) { this.write(how); return false; }
    const reply = await this.sends.answer(how === 'press', this.account, async digest => {
      const line = await this.command('digest ' + Buffer.from(digest).toString('hex'), 'signature');
      return Buffer.from(line.split(' ')[1], 'hex');
    });
    this.lamps.push(how === 'press' ? 1 : 2);
    if (how === 'press' && this.dropAfterSigning) {
      this.dropAfterSigning = false;
      this.drop();
    } else if (reply) this.speak(reply);
    return true;
  }

  private write(line: string) { this.child.stdin!.write(line + '\n'); }

  wait(...words: string[]) {
    return new Promise<string>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('BOARD_SILENT ' + words.join())), 10_000);
      this.waiting.push({words, resolve: line => { clearTimeout(timer); resolve(line); }});
    });
  }
  async command(line: string, ...answers: string[]) {
    if (this.sends.awaits && (line === 'press' || line === 'refuse' || line === 'wrongpin')) { await this.press(line); return 'ok'; }
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
    this.sends.reset();
    return await this.command(`connect ${epoch.toString('hex')} ${phone}`, 'open', 'denied') === 'open';
  }
  data(base64: string) {
    if (!this.connected) return;
    const {forward, prompt} = this.sends.phone(Buffer.from(base64, 'base64'));
    if (forward.length) this.write('data ' + Buffer.from(forward).toString('hex'));
    if (prompt) this.prompted();
  }
  disconnect() { if (this.connected) { this.connected = false; this.write('close'); } }
  stop() { this.child.stdin!.end(); this.child.kill(); }
}

export const epochOf = () => randomBytes(16);
