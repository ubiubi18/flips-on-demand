/** iDNA amounts arrive as integer base units (18 decimals) and are shown at the edge. */

const SCALE = 10n ** 18n;

export function formatAmount(baseUnits: string): string {
  let value: bigint;
  try {
    value = BigInt(baseUnits);
  } catch {
    return baseUnits;
  }
  const whole = value / SCALE;
  const fraction = value % SCALE;
  if (fraction === 0n) {
    return whole.toString();
  }
  const trimmed = fraction.toString().padStart(18, "0").replace(/0+$/, "");
  return `${whole}.${trimmed}`;
}

export function shortHex(value: string, keep = 6): string {
  if (value.length <= keep * 2 + 3) {
    return value;
  }
  return `${value.slice(0, keep + 2)}...${value.slice(-keep)}`;
}

export function formatTimestamp(value: string): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) {
    return value;
  }
  return new Date(parsed).toISOString().replace("T", " ").replace(".000Z", "Z");
}
