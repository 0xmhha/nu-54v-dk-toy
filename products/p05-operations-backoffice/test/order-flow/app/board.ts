// The board process (../board/board_host.c) and the phone's radio to it.
import {spawn, type ChildProcess} from 'node:child_process';
import {randomBytes} from 'node:crypto';
import type {PaymentToken} from '@nu54/protocol';
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
  /** The wallet account. Twenty zero bytes while the board has no owner: the key is made at the registration. */
  account = '';
  /** Whether the board advertises by itself for its next owner. */
  waitsForOwner = false;
  /** How many seconds the board's latest possible time is ahead of its earliest one; 0 without a grant. */
  ahead = 0;
  generation = 0n;
  connected = false;
  /** The next signature never reaches the phone: the link ends once the board signed. */
  dropAfterSigning = false;
  private deaf = false;
  /** Answers what the firmware does not know yet. */
  readonly sends: SendStandIn;
  readonly sounds = {fragment: new Set<(chunk: string) => void>(), lost: new Set<() => void>()};

  constructor(binary: string, token: PaymentToken) {
    this.sends = new SendStandIn(token);
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
    else if (word === 'account') this.account = '0x' + rest[0];
    else if (word === 'owner-wait') this.waitsForOwner = rest[0] === '1';
    else if (word === 'clock') this.ahead = Number(BigInt(rest[1]) - BigInt(rest[0]));
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
  /**
   * The owner resets the board with its own buttons, without a phone. Answers whether it erased itself: it refuses
   * while a session, a request or an approved order still needs it. A reset board needs the operator's grant again.
   */
  async factoryReset() { return (await this.command('factory', 'booted', 'lamp')).startsWith('booted'); }
  /** Power cycle. The board keeps the operator's grant and goes on from the time it saved last, so its clock is late. */
  boot() { return this.command('boot', 'booted'); }
  /** The operator installs a grant that runs for `life` seconds; `ahead` is how many seconds the signing took. */
  async grant(ahead = 2, life = 3000) {
    if (await this.command(`grant ${life} ${ahead}`, 'ok', 'refused') !== 'ok') throw new Error('BOARD_GRANT_REFUSED');
  }

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
