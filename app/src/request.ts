/**
 * Hands a typed action request to the user as a file. The request is the only
 * thing the interface produces for the CLI: it names what the user intends,
 * carries no key material, and never authorises a payment on its own.
 */
export function downloadActionRequest(request: Record<string, unknown>, fileName: string): void {
  const blob = new Blob([JSON.stringify(request, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = fileName;
  link.click();
  URL.revokeObjectURL(url);
}
