// The board keeps one Bluetooth connection, and a second connection ends the first. The board pages talk to the
// board in two ways: a session the customer started (a test, a return, a reset: it may wait up to 110 seconds for
// SW1 and the PIN), and the account proof behind the question "how many orders of this account are unfinished",
// which opens a connection of its own. Both take a turn here, so neither starts while the other is under way.
//
// Nothing that runs inside a turn may wait for another turn: it would wait for itself.

let last: Promise<unknown> = Promise.resolve();

/** Runs `run` once everything that asked for the board before it has ended, however it ended. */
export function boardTurn<T>(run: () => Promise<T>): Promise<T> {
  const turn = last.then(run, run);
  last = turn.catch(() => {});
  return turn;
}
