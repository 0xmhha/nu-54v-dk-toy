const formatter = (zone: string) =>
  new Intl.DateTimeFormat("en-GB", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  });
function wallTime(date: Date, format: Intl.DateTimeFormat) {
  const parts = Object.fromEntries(
    format.formatToParts(date).map((part) => [part.type, part.value])
  );
  return `${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}`;
}
export function localScheduleValue(instant: string, zone: string) {
  return wallTime(new Date(instant), formatter(zone));
}
export function scheduleCandidates(local: string, zone: string): string[] {
  if (!/^\d{4}-\d\d-\d\dT\d\d:\d\d$/.test(local)) return [];
  const naive = Date.parse(local + "Z");
  if (!Number.isFinite(naive)) return [];
  const format = formatter(zone);
  const difference = new Set<number>();
  // Sample both sides of a transition to discover UTC offset candidates, then
  // round-trip each candidate. A skipped time has no round-trip; a fold has two.
  for (const hours of [-36, -12, 0, 12, 36]) {
    const sample = naive + hours * 3600000;
    difference.add(
      Date.parse(wallTime(new Date(sample), format) + "Z") - sample
    );
  }
  return [...difference]
    .map((offset) => new Date(naive - offset))
    .filter((candidate) => wallTime(candidate, format) === local)
    .map((candidate) => candidate.toISOString())
    .sort();
}
