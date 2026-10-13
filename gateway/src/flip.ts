// The legacy flip content format, read the way the client's own renderer reads
// it (`SOURCES.md` F-98). A flip's decrypted content is two hex parts, each the
// RLP encoding of a list:
//
//   public  = RLP([image, image])                 the first half of the pictures
//   private = RLP([image, image, [order, order]]) the rest, then the two orders
//
// Each order names pictures by index, so a flip is four pictures and two
// sequences over them; the validator chooses the sequence that looks right. The
// same rules live in `proofs/flip-core` for the guest, and both read the frozen
// vector in `proofs/flip/vectors/flip-v1.json`.

const IMAGES = 4;
const ORDERS = 2;
const ORDER_ENTRIES = IMAGES;

export interface DecodedFlip {
  images: Buffer[];
  orders: number[][];
}

type Item = { kind: "bytes"; value: Buffer } | { kind: "list"; value: Buffer };

/** Decodes one hex part: the client writes `0x`-prefixed hex and reads both. */
function hexBytes(value: unknown, field: string): Buffer {
  if (typeof value !== "string") {
    throw new Error(`${field} must be a hex string`);
  }
  const digits = value.startsWith("0x") || value.startsWith("0X") ? value.slice(2) : value;
  if (digits.length % 2 !== 0 || !/^[0-9a-fA-F]*$/.test(digits)) {
    throw new Error(`${field} is not an even number of hex digits`);
  }
  return Buffer.from(digits, "hex");
}

/** One RLP item and how many bytes it took. Long forms carry their length. */
function itemAt(input: Buffer, wanted: number): { item: Item; consumed: number } | undefined {
  let at = 0;
  for (let index = 0; index <= wanted; index += 1) {
    const first = input[at];
    if (first === undefined) {
      return undefined;
    }
    let list = false;
    let length = 0;
    let prefix = 0;
    if (first <= 0x7f) {
      length = 1;
    } else if (first <= 0xb7) {
      length = first - 0x80;
      prefix = 1;
    } else if (first <= 0xbf) {
      const width = first - 0xb7;
      const declared = lengthAt(input, at, width);
      if (declared === undefined) {
        return undefined;
      }
      length = declared;
      prefix = 1 + width;
    } else if (first <= 0xf7) {
      list = true;
      length = first - 0xc0;
      prefix = 1;
    } else {
      const width = first - 0xf7;
      const declared = lengthAt(input, at, width);
      if (declared === undefined) {
        return undefined;
      }
      list = true;
      length = declared;
      prefix = 1 + width;
    }
    const start = at + prefix;
    const end = start + length;
    if (end > input.length) {
      return undefined;
    }
    const payload = input.subarray(start, end);
    if (index === wanted) {
      if (list) {
        return { item: { kind: "list", value: payload }, consumed: end };
      }
      // A byte that stands for itself is its own payload.
      return { item: { kind: "bytes", value: prefix === 0 ? input.subarray(at, end) : payload }, consumed: end };
    }
    at = end;
  }
  return undefined;
}

function lengthAt(input: Buffer, at: number, width: number): number | undefined {
  if (width === 0 || width > 6) {
    return undefined;
  }
  if (at + 1 + width > input.length || input[at + 1] === 0) {
    // No room for the length, or not the canonical form no client writes.
    return undefined;
  }
  let length = 0;
  for (let offset = 0; offset < width; offset += 1) {
    length = length * 256 + input[at + 1 + offset];
  }
  return length;
}

function items(input: Buffer): Item[] {
  const out: Item[] = [];
  let rest = input;
  while (rest.length > 0) {
    const found = itemAt(rest, 0);
    if (found === undefined) {
      throw new Error("the RLP is malformed");
    }
    out.push(found.item);
    rest = rest.subarray(found.consumed);
  }
  return out;
}

/** The payload of a top-level list, with nothing after it. */
function topList(input: Buffer): Buffer {
  const found = itemAt(input, 0);
  if (found === undefined || found.item.kind !== "list" || found.consumed !== input.length) {
    throw new Error("the part is not one RLP list");
  }
  return found.item.value;
}

/** One order entry's picture, or undefined when it names none. */
function indexOf(item: Item): number | undefined {
  const bytes = item.kind === "bytes" ? item.value : firstBytes(item.value);
  if (bytes === undefined) {
    return undefined;
  }
  // RLP writes the integer 0 as the empty string and 1..=0x7f as themselves.
  if (bytes.length === 0) {
    return 0;
  }
  if (bytes.length === 1 && bytes[0] <= 0x7f) {
    return bytes[0];
  }
  return undefined;
}

function firstBytes(list: Buffer): Buffer | undefined {
  const found = itemAt(list, 0);
  if (found === undefined || found.item.kind !== "bytes") {
    return undefined;
  }
  return found.item.value;
}

/** Reads one flip from its two hex parts, or says what is wrong with it. */
export function readFlip(publicPart: unknown, privatePart: unknown): DecodedFlip {
  const publicItems = items(topList(hexBytes(publicPart, "publicPart")));
  const images: Buffer[] = [];
  for (const item of publicItems) {
    if (item.kind !== "bytes") {
      throw new Error("the public part lists something that is not a picture");
    }
    images.push(item.value);
  }

  const privateItems = items(topList(hexBytes(privatePart, "privatePart")));
  let ordersList: Buffer | undefined;
  for (const item of privateItems) {
    if (item.kind === "bytes") {
      if (ordersList !== undefined) {
        throw new Error("the private part lists a picture after the orders");
      }
      images.push(item.value);
      continue;
    }
    if (ordersList !== undefined) {
      throw new Error("the private part lists more than one order list");
    }
    ordersList = item.value;
  }

  if (images.length !== IMAGES) {
    throw new Error(`the flip has ${images.length} pictures, expected ${IMAGES}`);
  }
  if (images.some((image) => image.length === 0)) {
    throw new Error("the flip has an empty picture");
  }
  if (ordersList === undefined) {
    throw new Error("the private part carries no orders");
  }
  const orderItems = items(ordersList);
  if (orderItems.length !== ORDERS) {
    throw new Error(`the flip has ${orderItems.length} orders, expected ${ORDERS}`);
  }
  const orders: number[][] = [];
  for (const order of orderItems) {
    if (order.kind !== "list") {
      throw new Error("an order is not a list");
    }
    const entries = items(order.value);
    if (entries.length !== ORDER_ENTRIES) {
      throw new Error(`an order has ${entries.length} entries, expected ${ORDER_ENTRIES}`);
    }
    const sequence = entries.map((entry) => {
      const index = indexOf(entry);
      if (index === undefined || index >= IMAGES) {
        throw new Error("an order names no picture of this flip");
      }
      return index;
    });
    orders.push(sequence);
  }
  if (orders[0].every((index, entry) => index === orders[1][entry])) {
    throw new Error("the flip's two orders are the same, so there is nothing to choose");
  }
  return { images, orders };
}

/** The media type of one picture, as the bytes themselves say. */
function mediaType(image: Buffer): string {
  if (image.length >= 8 && image.subarray(0, 8).equals(Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]))) {
    return "image/png";
  }
  if (image.length >= 3 && image[0] === 0xff && image[1] === 0xd8 && image[2] === 0xff) {
    return "image/jpeg";
  }
  if (image.length >= 6 && image.subarray(0, 6).toString("latin1").startsWith("GIF8")) {
    return "image/gif";
  }
  if (
    image.length >= 12 &&
    image.subarray(0, 4).toString("latin1") === "RIFF" &&
    image.subarray(8, 12).toString("latin1") === "WEBP"
  ) {
    return "image/webp";
  }
  // The client hands the browser image/png blobs and lets it sniff; a data URL
  // has to say something, so it says the same.
  return "image/png";
}

/** One picture as a data URL a participant's browser can show. */
export function imageDataUrl(image: Buffer): string {
  return `data:${mediaType(image)};base64,${image.toString("base64")}`;
}

/** The two sequences a participant chooses between, pictures in display order. */
export function variantsOf(flip: DecodedFlip): string[][] {
  return flip.orders.map((order) => order.map((index) => imageDataUrl(flip.images[index])));
}
