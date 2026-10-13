/**
 * One line after an action writes a file, saying what to do with it.
 *
 * The workflow is file-based (`SPEC.md` 4.3): the interface names what the user
 * wants and the local tool does the rest, so without this line the click looks
 * like it did nothing. It names the file and the command and stops there; the
 * FAQ holds the explanation.
 */
export function NextStep({ fileName, command }: { fileName: string | null; command: string }) {
  if (fileName === null) {
    return null;
  }
  return (
    <p className="notice" role="status">
      Saved request <span className="mono">{fileName}</span>. No transaction was sent. Next, use{" "}
      <span className="mono">{command}</span> with that file in your terminal.
    </p>
  );
}
