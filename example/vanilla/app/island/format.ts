export function stamp(date: Date): string {
  return date.toISOString().slice(11, 19);
}
