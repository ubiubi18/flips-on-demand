// Bundles the app into one self-contained HTML file.
//
// The script and stylesheet are inlined, so the local-file market reader works
// from a file:// URL. Optional session and public-preview features use network.

import { build } from "esbuild";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
// The release builder supplies a staging path; ordinary builds keep the same output.
const output = process.argv[2] ? resolve(process.argv[2]) : join(here, "dist", "flips-on-demand.html");

const result = await build({
  entryPoints: [join(here, "src", "main.tsx")],
  bundle: true,
  write: false,
  format: "iife",
  target: "es2020",
  minify: true,
  define: { "process.env.NODE_ENV": '"production"' },
  logLevel: "warning",
});

const script = result.outputFiles[0].text;
const styles = await readFile(join(here, "src", "styles.css"), "utf8");
const template = await readFile(join(here, "index.html"), "utf8");

const html = template
  .replace("/*STYLES*/", () => styles)
  .replace("/*SCRIPT*/", () => script);

await mkdir(dirname(output), { recursive: true });
await writeFile(output, html);
console.log(`wrote ${output} (${html.length} bytes)`);
