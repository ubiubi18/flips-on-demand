//! The legacy flip content format, as the client's own renderer reads it.
//!
//! A flip's decrypted content is two RLP byte strings. JSON-RPC represents them
//! as `0x` hex in transit, but `hexutil.Bytes` decodes that transport wrapper
//! before the node encrypts the parts:
//!
//! ```text
//!   public  = RLP([[image, image]])
//!   private = RLP([[image, image], [order, order]])
//! ```
//!
//! Each `order` is a sequence whose entries name an image by index: the client
//! renders `images[order[i]]` in order, so a flip is four images and two
//! sequences over them. The renderer accepts an entry that is either the index
//! byte itself or a list whose first element is the index; both forms appear in
//! the wild, and `SOURCES.md` F-98 records the two client files that define and
//! read this.
//!
//! This crate is deliberately separate from `fod-registration-core`: that crate
//! is compiled into the version 1 registration guest, and adding anything to it
//! moves that guest's image id and invalidates the seal the escrow ships. The
//! version 2 program has its own id, and it is the one that checks content.
//!
//! It is `no_std` and allocation-free: the caller decodes the hex into its own
//! buffer (`hex_bytes_len`, `decode_hex`) and hands the bytes here.

#![no_std]

/// Pictures in one flip. The client's `FLIP_LENGTH`.
pub const IMAGES: usize = 4;
/// Orders in one flip: the two sequences a validator chooses between.
pub const ORDERS: usize = 2;
/// Entries in one order.
pub const ORDER_ENTRIES: usize = IMAGES;

/// Why a part is not a flip this release sells. Each variant names what the
/// renderer would have to cope with instead.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Refusal {
    /// Not hex, odd length, or not the RLP list the format starts with.
    Malformed,
    /// The parts do not carry exactly [`IMAGES`] pictures.
    ImageCount,
    /// A picture with no bytes, which no renderer can show.
    EmptyImage,
    /// Fewer or more than [`ORDERS`] sequences.
    OrderCount,
    /// A sequence that is not [`ORDER_ENTRIES`] long.
    OrderLength,
    /// A sequence entry that names no picture in this flip.
    OrderIndex,
    /// Both sequences are the same, so there is nothing to choose between.
    Ambiguous,
}

/// One sequence over the flip's pictures.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Order<'a> {
    entries: &'a [u8],
}

impl<'a> Order<'a> {
    /// The picture this entry names, in order.
    pub fn index(&self, entry: usize) -> Option<u8> {
        let (item, consumed) = item_at(self.entries, entry)?;
        let bytes = match item {
            Item::Bytes(bytes) => bytes,
            Item::List(list) => first_bytes(list)?,
        };
        // RLP writes the integer 0 as the empty string and 1..=0x7f as
        // themselves, which is why the client's decoder reads the first byte and
        // defaults to 0 when there is none.
        let _ = consumed;
        match bytes {
            [] => Some(0),
            [value] if *value <= 0x7f => Some(*value),
            _ => None,
        }
    }

    /// How many entries the sequence has.
    pub fn len(&self) -> usize {
        count_items_exact(self.entries).unwrap_or(0)
    }

    /// Whether the sequence has no entries.
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }
}

/// A flip's four pictures and two orders.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Flip<'a> {
    pub images: [&'a [u8]; IMAGES],
    pub orders: [Order<'a>; ORDERS],
}

/// One RLP item: a byte string or a list of them.
#[derive(Debug, Clone, Copy)]
enum Item<'a> {
    Bytes(&'a [u8]),
    List(&'a [u8]),
}

/// The bytes a hex part carries, or why it cannot be one. `text` may carry the
/// `0x` prefix the client writes.
pub fn hex_bytes_len(text: &[u8]) -> Result<usize, Refusal> {
    let digits = strip_prefix(text);
    if digits.len() % 2 != 0 {
        return Err(Refusal::Malformed);
    }
    if !digits.iter().all(|byte| hex_value(*byte).is_some()) {
        return Err(Refusal::Malformed);
    }
    Ok(digits.len() / 2)
}

/// Decodes one hex part into `out`, which must be [`hex_bytes_len`] long.
pub fn decode_hex(text: &[u8], out: &mut [u8]) -> Result<(), Refusal> {
    let digits = strip_prefix(text);
    if out.len() != digits.len() / 2 {
        return Err(Refusal::Malformed);
    }
    for (pair, slot) in digits.chunks_exact(2).zip(out.iter_mut()) {
        let high = hex_value(pair[0]).ok_or(Refusal::Malformed)?;
        let low = hex_value(pair[1]).ok_or(Refusal::Malformed)?;
        *slot = (high << 4) | low;
    }
    Ok(())
}

fn append_images<'a>(
    mut encoded: &'a [u8],
    images: &mut [&'a [u8]; IMAGES],
    count: &mut usize,
) -> Result<(), Refusal> {
    while !encoded.is_empty() {
        let (item, consumed) = item_at(encoded, 0).ok_or(Refusal::Malformed)?;
        let bytes = match item {
            Item::Bytes(bytes) => bytes,
            Item::List(_) => return Err(Refusal::Malformed),
        };
        if *count == IMAGES {
            return Err(Refusal::ImageCount);
        }
        images[*count] = bytes;
        *count += 1;
        encoded = encoded.get(consumed..).ok_or(Refusal::Malformed)?;
    }
    Ok(())
}

/// Reads one flip from the node's two decrypted RLP parts. The renderer's
/// current nested image lists are canonical. The earlier flat image-list shape
/// remains accepted for already recorded fixtures and legacy clients.
pub fn read<'a>(public_part: &'a [u8], private_part: &'a [u8]) -> Result<Flip<'a>, Refusal> {
    let mut images: [&'a [u8]; IMAGES] = [&[]; IMAGES];
    let mut count = 0usize;

    let public = top_list(public_part)?;
    match item_at(public, 0) {
        Some((Item::List(nested), consumed)) if consumed == public.len() => {
            append_images(nested, &mut images, &mut count)?;
        }
        _ => append_images(public, &mut images, &mut count)?,
    }

    let private = top_list(private_part)?;
    let mut orders_at: Option<&'a [u8]> = None;
    match item_at(private, 0) {
        Some((Item::List(nested_images), consumed)) => {
            append_images(nested_images, &mut images, &mut count)?;
            let remainder = private.get(consumed..).ok_or(Refusal::Malformed)?;
            let (orders, orders_bytes) = item_at(remainder, 0).ok_or(Refusal::OrderCount)?;
            if orders_bytes != remainder.len() {
                return Err(Refusal::OrderCount);
            }
            orders_at = match orders {
                Item::List(list) => Some(list),
                Item::Bytes(_) => return Err(Refusal::OrderCount),
            };
        }
        Some((Item::Bytes(_), _)) => {
            let mut remainder = private;
            while !remainder.is_empty() {
                let (item, consumed) = item_at(remainder, 0).ok_or(Refusal::Malformed)?;
                match item {
                    Item::Bytes(bytes) => {
                        if orders_at.is_some() {
                            return Err(Refusal::Malformed);
                        }
                        if count == IMAGES {
                            return Err(Refusal::ImageCount);
                        }
                        images[count] = bytes;
                        count += 1;
                    }
                    Item::List(list) => {
                        if orders_at.is_some() {
                            return Err(Refusal::OrderCount);
                        }
                        orders_at = Some(list);
                    }
                }
                remainder = remainder.get(consumed..).ok_or(Refusal::Malformed)?;
            }
        }
        None => return Err(Refusal::Malformed),
    }

    if count != IMAGES {
        return Err(Refusal::ImageCount);
    }
    for image in images {
        if image.is_empty() {
            return Err(Refusal::EmptyImage);
        }
    }

    let orders_bytes = orders_at.ok_or(Refusal::OrderCount)?;
    if count_items_exact(orders_bytes) != Some(ORDERS) {
        return Err(Refusal::OrderCount);
    }
    let mut orders = [Order { entries: &[] }; ORDERS];
    for (slot, order) in orders.iter_mut().enumerate() {
        let (item, _) = item_at(orders_bytes, slot).ok_or(Refusal::OrderCount)?;
        let entries = match item {
            Item::List(list) => list,
            Item::Bytes(_) => return Err(Refusal::OrderLength),
        };
        if count_items_exact(entries) != Some(ORDER_ENTRIES) {
            return Err(Refusal::OrderLength);
        }
        *order = Order { entries };
        for entry in 0..ORDER_ENTRIES {
            if order.index(entry).ok_or(Refusal::OrderIndex)? as usize >= IMAGES {
                return Err(Refusal::OrderIndex);
            }
        }
    }
    if same_order(&orders[0], &orders[1]) {
        return Err(Refusal::Ambiguous);
    }
    Ok(Flip { images, orders })
}

fn same_order(left: &Order<'_>, right: &Order<'_>) -> bool {
    (0..ORDER_ENTRIES).all(|entry| left.index(entry) == right.index(entry))
}

fn strip_prefix(text: &[u8]) -> &[u8] {
    if text.len() >= 2 && text[0] == b'0' && (text[1] == b'x' || text[1] == b'X') {
        return &text[2..];
    }
    text
}

fn hex_value(byte: u8) -> Option<u8> {
    match byte {
        b'0'..=b'9' => Some(byte - b'0'),
        b'a'..=b'f' => Some(byte - b'a' + 10),
        b'A'..=b'F' => Some(byte - b'A' + 10),
        _ => None,
    }
}

/// The payload of one RLP list, with nothing after it: the client's decoder
/// refuses a trailing remainder, and so does this one.
fn top_list(input: &[u8]) -> Result<&[u8], Refusal> {
    match item_at(input, 0) {
        Some((Item::List(list), consumed)) if consumed == input.len() => Ok(list),
        _ => Err(Refusal::Malformed),
    }
}

/// One item and how many bytes it took. `0x00..=0x7f` is the byte itself;
/// `0x80..=0xb7` and `0xc0..=0xf7` carry their length; the longer forms carry
/// it big-endian after the marker.
fn item_at(input: &[u8], wanted: usize) -> Option<(Item<'_>, usize)> {
    let mut at = 0usize;
    for index in 0..=wanted {
        let first = *input.get(at)?;
        let (list, length, prefix) = match first {
            0x00..=0x7f => (false, 1usize, 0usize),
            0x80..=0xb7 => {
                let length = (first - 0x80) as usize;
                if length == 1 && *input.get(at + 1)? < 0x80 {
                    return None;
                }
                (false, length, 1)
            }
            0xb8..=0xbf => {
                let width = (first - 0xb7) as usize;
                let length = length_at(input, at, width)?;
                if length < 56 {
                    return None;
                }
                (false, length, 1 + width)
            }
            0xc0..=0xf7 => (true, (first - 0xc0) as usize, 1),
            _ => {
                let width = (first - 0xf7) as usize;
                let length = length_at(input, at, width)?;
                if length < 56 {
                    return None;
                }
                (true, length, 1 + width)
            }
        };
        let start = at.checked_add(prefix)?;
        let end = start.checked_add(length)?;
        let payload = input.get(start..end)?;
        if index == wanted {
            let item = if list {
                Item::List(payload)
            } else if prefix == 0 {
                // A byte that stands for itself is its own payload.
                Item::Bytes(&input[at..end])
            } else {
                Item::Bytes(payload)
            };
            return Some((item, end));
        }
        at = end;
    }
    None
}

/// The big-endian length that follows a long-form marker.
fn length_at(input: &[u8], at: usize, width: usize) -> Option<usize> {
    if width == 0 || width > core::mem::size_of::<usize>() {
        return None;
    }
    let bytes = input.get(at + 1..at + 1 + width)?;
    if bytes[0] == 0 {
        // Not a canonical length: no client writes a leading zero.
        return None;
    }
    let mut length = 0usize;
    for byte in bytes {
        length = length.checked_mul(256)?.checked_add(*byte as usize)?;
    }
    Some(length)
}

fn count_items_exact(mut input: &[u8]) -> Option<usize> {
    let mut count = 0;
    while !input.is_empty() {
        let (_, consumed) = item_at(input, 0)?;
        input = input.get(consumed..)?;
        count += 1;
    }
    Some(count)
}

/// The first byte string of a list, which is how an order entry names a
/// picture when it is written as a list.
fn first_bytes(list: &[u8]) -> Option<&[u8]> {
    count_items_exact(list)?;
    match item_at(list, 0)? {
        (Item::Bytes(bytes), _) => Some(bytes),
        (Item::List(_), _) => None,
    }
}

#[cfg(test)]
mod tests {
    extern crate std;

    use std::prelude::v1::*;

    use super::*;

    fn vector() -> serde_json::Value {
        let path = concat!(env!("CARGO_MANIFEST_DIR"), "/../flip/vectors/flip-v1.json");
        let raw = std::fs::read_to_string(path).expect("read the frozen flip vector");
        serde_json::from_str(&raw).expect("parse the frozen flip vector")
    }

    fn decoded(value: &str) -> Vec<u8> {
        let text = value.as_bytes();
        let mut out = vec![0u8; hex_bytes_len(text).expect("hex")];
        decode_hex(text, &mut out).expect("decode");
        out
    }

    // The frozen vector is encoded by the `rlp` package the client itself uses,
    // so this is the wire form of the renderer, not ours (`SOURCES.md` F-98).
    #[test]
    fn the_frozen_vector_is_a_flip() {
        let vector = vector();
        let public = decoded(vector["publicPart"].as_str().expect("public part"));
        let private = decoded(vector["privatePart"].as_str().expect("private part"));
        let flip = read(&public, &private).expect("the frozen vector is a flip");
        for (index, image) in vector["images"]
            .as_array()
            .expect("images")
            .iter()
            .enumerate()
        {
            let expected = decoded(image.as_str().expect("image"));
            assert_eq!(flip.images[index], expected, "image {index}");
        }
        for (order, expected) in flip
            .orders
            .iter()
            .zip(vector["orders"].as_array().expect("orders"))
        {
            let expected = expected.as_array().expect("order");
            assert_eq!(order.len(), expected.len());
            for (entry, value) in expected.iter().enumerate() {
                assert_eq!(
                    order.index(entry),
                    Some(value.as_u64().expect("index") as u8),
                    "order entry {entry}"
                );
            }
        }
    }

    fn flip_parts(
        images: [&[u8]; IMAGES],
        orders: [[u8; IMAGES]; 2],
        public_images: usize,
    ) -> (Vec<u8>, Vec<u8>) {
        let encode_bytes = |value: &[u8]| {
            let mut out = vec![0x80 + value.len() as u8];
            out.extend_from_slice(value);
            out
        };
        let encode_list = |payload: &[u8]| {
            let mut out = vec![0xc0 + payload.len() as u8];
            out.extend_from_slice(payload);
            out
        };
        let mut public = Vec::new();
        for image in images.iter().take(public_images) {
            public.extend(encode_bytes(image));
        }
        let mut private = Vec::new();
        for image in images.iter().skip(public_images) {
            private.extend(encode_bytes(image));
        }
        let mut order_payload = Vec::new();
        for order in orders {
            let mut inner = Vec::new();
            for value in order {
                // RLP writes the integer 0 as the empty string, and 1..=0x7f as
                // themselves; the same rule the client's encoder applies.
                if value == 0 {
                    inner.push(0x80);
                } else {
                    inner.push(value);
                }
            }
            order_payload.extend(encode_list(&inner));
        }
        private.extend(encode_list(&order_payload));
        (encode_list(&public), encode_list(&private))
    }

    const PICTURES: [&[u8]; IMAGES] = [b"one", b"two", b"three", b"four"];
    const ORDERS: [[u8; IMAGES]; 2] = [[0, 1, 2, 3], [2, 0, 3, 1]];

    #[test]
    fn a_well_formed_flip_reads() {
        let (public, private) = flip_parts(PICTURES, ORDERS, 2);
        let flip = read(&public, &private).expect("a flip");
        assert_eq!(flip.images[0], PICTURES[0]);
        assert_eq!(flip.images[3], PICTURES[3]);
        assert_eq!(flip.orders[1].index(0), Some(2));
    }

    #[test]
    fn the_renderers_nested_parts_read() {
        let encode_bytes = |value: &[u8]| {
            let mut out = vec![0x80 + value.len() as u8];
            out.extend_from_slice(value);
            out
        };
        let encode_list = |payload: &[u8]| {
            let mut out = vec![0xc0 + payload.len() as u8];
            out.extend_from_slice(payload);
            out
        };

        let mut public_images = Vec::new();
        for image in PICTURES.iter().take(2) {
            public_images.extend(encode_bytes(image));
        }
        let public = encode_list(&encode_list(&public_images));

        let mut private_images = Vec::new();
        for image in PICTURES.iter().skip(2) {
            private_images.extend(encode_bytes(image));
        }
        let mut orders = Vec::new();
        for order in ORDERS {
            let mut entries = Vec::new();
            for value in order {
                entries.push(if value == 0 { 0x80 } else { value });
            }
            orders.extend(encode_list(&entries));
        }
        let mut private_payload = encode_list(&private_images);
        private_payload.extend(encode_list(&orders));
        let private = encode_list(&private_payload);

        let flip = read(&public, &private).expect("the renderer's nested flip");
        assert_eq!(flip.images, PICTURES);
        assert_eq!(flip.orders[0].index(0), Some(0));
        assert_eq!(flip.orders[1].index(3), Some(1));
    }

    #[test]
    fn a_split_that_is_not_half_and_half_still_reads() {
        let (public, private) = flip_parts(PICTURES, ORDERS, 1);
        assert!(read(&public, &private).is_ok());
    }

    #[test]
    fn three_pictures_are_refused() {
        let (public, private) = flip_parts([PICTURES[0], PICTURES[1], PICTURES[2], b""], ORDERS, 2);
        assert_eq!(read(&public, &private), Err(Refusal::EmptyImage));
        let (public, private) = flip_parts(PICTURES, ORDERS, 3);
        let mut short = private.clone();
        short.truncate(short.len() - 4);
        assert!(matches!(
            read(&public, &short),
            Err(Refusal::Malformed) | Err(Refusal::ImageCount)
        ));
    }

    #[test]
    fn an_index_outside_the_pictures_is_refused() {
        let (public, private) = flip_parts(PICTURES, [[0, 1, 2, 4], [1, 0, 3, 2]], 2);
        assert_eq!(read(&public, &private), Err(Refusal::OrderIndex));
    }

    #[test]
    fn identical_orders_are_refused() {
        let (public, private) = flip_parts(PICTURES, [[0, 1, 2, 3], [0, 1, 2, 3]], 2);
        assert_eq!(read(&public, &private), Err(Refusal::Ambiguous));
    }

    #[test]
    fn a_trailing_remainder_is_refused() {
        let (mut public, private) = flip_parts(PICTURES, ORDERS, 2);
        public.push(0x01);
        assert_eq!(read(&public, &private), Err(Refusal::Malformed));
    }

    #[test]
    fn malformed_bytes_inside_orders_are_refused() {
        let (public, private) = flip_parts(PICTURES, ORDERS, 2);
        let orders_at = private
            .iter()
            .position(|byte| *byte == 0xca)
            .expect("orders list");

        let mut after_orders = private.clone();
        after_orders[0] += 1;
        after_orders[orders_at] += 1;
        after_orders.push(0xff);
        assert_eq!(read(&public, &after_orders), Err(Refusal::OrderCount));

        let mut after_entries = private;
        after_entries[0] += 1;
        after_entries[orders_at] += 1;
        after_entries[orders_at + 1] += 1;
        after_entries.insert(orders_at + 1 + 5, 0xff);
        assert_eq!(read(&public, &after_entries), Err(Refusal::OrderLength));
    }

    #[test]
    fn noncanonical_rlp_is_refused_like_the_node_decoder() {
        // 0x81 0x43 encodes a single byte with a needless length prefix.
        let public = [0xc2, 0x41, 0x42];
        let private = [
            0xce, 0x81, 0x43, 0x44, 0xca, 0xc4, 0x80, 0x01, 0x02, 0x03, 0xc4, 0x03, 0x02, 0x01,
            0x80,
        ];
        assert_eq!(read(&public, &private), Err(Refusal::Malformed));
        assert!(item_at(&[0xb8, 0x01, 0x41], 0).is_none());
        assert!(item_at(&[0xf8, 0x01, 0x80], 0).is_none());
    }

    #[test]
    fn odd_or_non_hex_parts_are_refused() {
        assert_eq!(hex_bytes_len(b"0x123"), Err(Refusal::Malformed));
        assert_eq!(hex_bytes_len(b"0x12zz"), Err(Refusal::Malformed));
        assert_eq!(hex_bytes_len(b"0x12"), Ok(1));
    }
}
