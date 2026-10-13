// The gateway reads flip content the way the client's renderer does
// (`SOURCES.md` F-98), and the frozen vector is the cross-language check: it was
// encoded with the same `rlp` package the client uses, so a decoder that reads
// it correctly reads the client's format.

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { imageDataUrl, readFlip, variantsOf } from "../src/flip.ts";
import { loadPool } from "../src/pool.ts";

const vectorPath = join(import.meta.dirname, "..", "..", "proofs", "flip", "vectors", "flip-v1.json");

interface Vector {
  images: string[];
  orders: number[][];
  publicPart: string;
  privatePart: string;
}

async function vector(): Promise<Vector> {
  return JSON.parse(await readFile(vectorPath, "utf8")) as Vector;
}

describe("the legacy flip format", () => {
  it("reads the frozen vector the client encoded", async () => {
    const frozen = await vector();
    const flip = readFlip(frozen.publicPart, frozen.privatePart);
    assert.equal(flip.images.length, 4);
    assert.deepEqual(
      flip.images.map((image) => image.toString("hex")),
      frozen.images
    );
    assert.deepEqual(flip.orders, frozen.orders);
  });

  it("renders each order as the sequence of pictures it names", async () => {
    const frozen = await vector();
    const flip = readFlip(frozen.publicPart, frozen.privatePart);
    const variants = variantsOf(flip);
    assert.equal(variants.length, 2);
    for (const [variant, sequence] of variants.entries()) {
      const expected = flip.orders[variant].map((index) => imageDataUrl(flip.images[index]));
      assert.deepEqual(sequence, expected);
      assert.equal(sequence.length, 4);
    }
    // The picture a sequence puts first is what tells the two apart.
    assert.notEqual(variants[0][0], variants[1][0]);
  });

  it("refuses what is not the format", async () => {
    const frozen = await vector();
    const flip = readFlip(frozen.publicPart, frozen.privatePart);

    assert.throws(() => readFlip("0x123", frozen.privatePart), /hex digits/);
    assert.throws(() => readFlip("0x12zz", frozen.privatePart), /hex digits/);
    assert.throws(() => readFlip(frozen.publicPart, frozen.privatePart + "01"), /one RLP list/);
    assert.throws(() => readFlip("0x0a", frozen.privatePart), /one RLP list/);

    // Three pictures: a public part that lists one picture instead of two.
    const shortPublic = Buffer.concat([
      Buffer.from([0xd0, 0x8f]),
      Buffer.from(frozen.images[0], "hex"),
    ]);
    assert.throws(
      () => readFlip(`0x${shortPublic.toString("hex")}`, frozen.privatePart),
      /pictures/
    );

    // An index that names no picture.
    const orders = Buffer.from(frozen.privatePart.slice(2), "hex");
    const tampered = Buffer.from(orders);
    tampered[tampered.length - 1] = 0x09;
    assert.throws(() => readFlip(frozen.publicPart, `0x${tampered.toString("hex")}`), /no picture/);

    // The same order twice is nothing to choose between.
    const same = Buffer.from(orders);
    const secondOrderAt = same.length - 5;
    same[secondOrderAt + 1] = 0x80;
    same[secondOrderAt + 2] = 0x01;
    same[secondOrderAt + 3] = 0x02;
    same[secondOrderAt + 4] = 0x03;
    assert.throws(() => readFlip(frozen.publicPart, `0x${same.toString("hex")}`), /nothing to choose/);

    assert.equal(flip.images[0].toString("hex"), frozen.images[0]);
  });

  it("names the picture type from its own bytes", () => {
    const png = Buffer.from("89504e470d0a1a0a00000000", "hex");
    const jpeg = Buffer.from("ffd8ffe000104a464946", "hex");
    assert.match(imageDataUrl(png), /^data:image\/png;base64,/);
    assert.match(imageDataUrl(jpeg), /^data:image\/jpeg;base64,/);
  });

  it("loads the example pool's flip as eight real pictures", async () => {
    const pool = await loadPool(join(import.meta.dirname, "..", "example-pool.json"));
    const challenge = pool.challenges.find((entry) => entry.challengeId === "c1");
    assert.equal(challenge?.variants?.length, 2);
    for (const sequence of challenge?.variants ?? []) {
      assert.equal(sequence.length, 4);
      for (const image of sequence) {
        const bytes = Buffer.from(image.slice(image.indexOf(",") + 1), "base64");
        // The example carries real PNGs, so a participant's browser shows them.
        assert.equal(bytes.subarray(0, 8).toString("hex"), "89504e470d0a1a0a");
      }
    }
  });

  // The other direction: this file was written by `fod session-pool`, and the
  // Go test pins it byte for byte, so the CLI and this loader cannot drift.
  it("loads the pool the CLI wrote", async () => {
    const pool = await loadPool(join(import.meta.dirname, "..", "fixtures", "cli-pool.json"));
    assert.equal(pool.challenges.length, 1);
    const challenge = pool.challenges[0];
    assert.equal(challenge.answerDigest.length, 64);
    assert.equal(challenge.variants?.length, 2);
    for (const sequence of challenge.variants ?? []) {
      assert.equal(sequence.length, 4);
      assert.ok(sequence.every((image) => image.startsWith("data:image/")));
    }
  });

  // The digest is what a utility pins when it decides which session it
  // accepts: the same name with different content has to be a different value.
  it("pins a session by a digest of what it is", async () => {
    const path = join(import.meta.dirname, "..", "example-pool.json");
    const pool = await loadPool(path);
    assert.match(pool.contentDigest, /^[0-9a-f]{64}$/);
    assert.equal((await loadPool(path)).contentDigest, pool.contentDigest, "the digest is stable");

    const directory = await mkdtemp(join(tmpdir(), "fod-pool-"));
    let written = 0;
    const variant = async (change: (doc: Record<string, unknown>) => void): Promise<string> => {
      const doc = JSON.parse(await readFile(path, "utf8")) as Record<string, unknown>;
      change(doc);
      const target = join(directory, `pool-${(written += 1)}.json`);
      await writeFile(target, JSON.stringify(doc));
      return (await loadPool(target)).contentDigest;
    };

    const otherAnswer = await variant((doc) => {
      const challenges = doc.challenges as { answerDigest: string }[];
      challenges[0].answerDigest = "00".repeat(32);
    });
    assert.notEqual(otherAnswer, pool.contentDigest, "the answer a challenge scores against is part of it");

    const otherRules = await variant((doc) => {
      const rules = doc.rules as { passThreshold: number };
      rules.passThreshold = 1;
    });
    assert.notEqual(otherRules, pool.contentDigest, "the pass threshold is part of it");

    const otherSource = await variant((doc) => {
      const provenance = doc.provenance as { source: string };
      provenance.source = "another source";
    });
    assert.notEqual(otherSource, pool.contentDigest, "the stated provenance is part of it");
  });
});
