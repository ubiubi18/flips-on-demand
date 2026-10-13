//! Synthetic feasibility spike for fast, buyer-bound encrypted content delivery.
//!
//! This is not a deployable escrow implementation. The registration proof must
//! still establish that the content key derived from `M` opens an authenticated
//! re-encryption of every original flip. A buyer can decrypt an unincluded
//! delivery transaction, so arbitrary censorship still defeats fair exchange.

#![no_std]

#[cfg(feature = "content")]
extern crate alloc;

#[cfg(feature = "content")]
pub mod content;

use k256::elliptic_curve::sec1::{FromEncodedPoint, ToEncodedPoint};
#[cfg(feature = "prover")]
use k256::elliptic_curve::Field;
use k256::elliptic_curve::PrimeField;
use k256::{AffinePoint, FieldBytes, ProjectivePoint, Scalar};
#[cfg(feature = "prover")]
use rand_core::OsRng;
use sha2::{Digest, Sha256};

#[cfg(test)]
const H_DST: &[u8] = b"FOD_FAST_DELIVERY_H_SECP256K1_XMD:SHA-256_SSWU_RO_V1";
/// Derived by the secp256k1_XMD:SHA-256_SSWU_RO_ suite of RFC 9380 using the
/// domain and message in `the_generator_pin_uses_hash_to_curve` below.
const H_BYTES: PointBytes = [
    0x03, 0x94, 0x78, 0xcc, 0x1a, 0x30, 0x11, 0xf6, 0x9d, 0xd3, 0x2c, 0xf7, 0xdc, 0x0b, 0x6c, 0xd6,
    0x00, 0x1f, 0x6a, 0x11, 0xe9, 0x5a, 0x37, 0x84, 0x00, 0xdf, 0x2f, 0xb3, 0xf8, 0xc3, 0x78, 0x45,
    0xd2,
];
const CHALLENGE_DOMAIN: &[u8] = b"fod:fast-delivery:proof:v1";
const KEY_DOMAIN: &[u8] = b"fod:fast-delivery:content-key:v1";
const ORDER_DOMAIN: &[u8] = b"fod:fast-delivery:order:v1";
const MAX_CONTEXT_BYTES: usize = 256;

pub type PointBytes = [u8; 33];

/// The seller keeps `message` and `blind` private. A separate content proof
/// must bind the resulting commitment to the published encrypted flip bytes.
#[cfg(feature = "prover")]
pub struct SellerKey {
    message: ProjectivePoint,
    blind: Scalar,
}

/// A Schnorr representation proof for R = rG and T - C = rB - bH.
/// `R || T || U || V || z_r || z_b` is 196 bytes in compressed form.
#[derive(Clone, Copy)]
pub struct Delivery {
    pub r_point: PointBytes,
    pub t_point: PointBytes,
    pub u_point: PointBytes,
    pub v_point: PointBytes,
    pub z_r: [u8; 32],
    pub z_b: [u8; 32],
}

pub const DELIVERY_BYTES: usize = 4 * 33 + 2 * 32;

impl Delivery {
    pub fn encode(&self) -> [u8; DELIVERY_BYTES] {
        let mut out = [0u8; DELIVERY_BYTES];
        out[0..33].copy_from_slice(&self.r_point);
        out[33..66].copy_from_slice(&self.t_point);
        out[66..99].copy_from_slice(&self.u_point);
        out[99..132].copy_from_slice(&self.v_point);
        out[132..164].copy_from_slice(&self.z_r);
        out[164..196].copy_from_slice(&self.z_b);
        out
    }

    pub fn decode(bytes: &[u8]) -> Option<Self> {
        if bytes.len() != DELIVERY_BYTES {
            return None;
        }
        Some(Self {
            r_point: bytes[0..33].try_into().ok()?,
            t_point: bytes[33..66].try_into().ok()?,
            u_point: bytes[66..99].try_into().ok()?,
            v_point: bytes[99..132].try_into().ok()?,
            z_r: bytes[132..164].try_into().ok()?,
            z_b: bytes[164..196].try_into().ok()?,
        })
    }
}

fn h_generator() -> ProjectivePoint {
    point_from_bytes(&H_BYTES).expect("pinned independent generator")
}

fn point_bytes(point: ProjectivePoint) -> PointBytes {
    let bytes = point.to_affine().to_encoded_point(true);
    bytes
        .as_bytes()
        .try_into()
        .expect("compressed secp256k1 point")
}

fn point_from_bytes(bytes: &PointBytes) -> Option<ProjectivePoint> {
    let encoded = k256::EncodedPoint::from_bytes(bytes).ok()?;
    let point = Option::<AffinePoint>::from(AffinePoint::from_encoded_point(&encoded))?;
    let point = ProjectivePoint::from(point);
    if point == ProjectivePoint::IDENTITY {
        return None;
    }
    Some(point)
}

pub fn valid_point(bytes: &PointBytes) -> bool {
    point_from_bytes(bytes).is_some()
}

fn scalar_from_bytes(bytes: &[u8; 32]) -> Option<Scalar> {
    Option::<Scalar>::from(Scalar::from_repr(FieldBytes::from(*bytes)))
}

fn challenge(context: &[u8], points: &[PointBytes; 6]) -> Scalar {
    let mut counter = 0u32;
    loop {
        let mut hash = Sha256::new();
        hash.update(CHALLENGE_DOMAIN);
        hash.update((context.len() as u32).to_be_bytes());
        hash.update(context);
        for point in points {
            hash.update(point);
        }
        hash.update(counter.to_be_bytes());
        let digest: [u8; 32] = hash.finalize().into();
        if let Some(scalar) = scalar_from_bytes(&digest) {
            return scalar;
        }
        counter = counter
            .checked_add(1)
            .expect("challenge rejection overflow");
    }
}

#[cfg(feature = "prover")]
impl SellerKey {
    pub fn generate() -> Self {
        let message_scalar = nonzero_scalar();
        let blind = nonzero_scalar();
        Self {
            message: ProjectivePoint::GENERATOR * message_scalar,
            blind,
        }
    }

    /// Restore private material held by the seller between authoring and
    /// delivery. The caller must protect these bytes like an encryption key.
    pub fn from_witness(message: &PointBytes, blind: &[u8; 32]) -> Option<Self> {
        let message = point_from_bytes(message)?;
        let blind = scalar_from_bytes(blind)?;
        if blind == Scalar::ZERO || message + h_generator() * blind == ProjectivePoint::IDENTITY {
            return None;
        }
        Some(Self { message, blind })
    }

    pub fn commitment(&self) -> PointBytes {
        point_bytes(self.message + h_generator() * self.blind)
    }

    pub fn content_key(&self) -> [u8; 32] {
        content_key(self.message)
    }

    /// Private witness fields for a pre-session content proof. Keep both
    /// values off chain and out of logs.
    pub fn witness_material(&self) -> (PointBytes, [u8; 32]) {
        (point_bytes(self.message), self.blind.to_bytes().into())
    }

    /// `context` must encode network, contract, bundle, funded order id and
    /// nonce, buyer address and delivery public key without ambiguity. The
    /// on-chain verifier must use exactly those stored values.
    pub fn deliver(&self, buyer: &PointBytes, context: &[u8]) -> Option<Delivery> {
        if context.is_empty() || context.len() > MAX_CONTEXT_BYTES {
            return None;
        }
        let buyer = point_from_bytes(buyer)?;
        let r = nonzero_scalar();
        let a = nonzero_scalar();
        let d = nonzero_scalar();
        Some(self.deliver_with_randomness(buyer, context, r, a, d))
    }

    fn deliver_with_randomness(
        &self,
        buyer: ProjectivePoint,
        context: &[u8],
        r: Scalar,
        a: Scalar,
        d: Scalar,
    ) -> Delivery {
        let h = h_generator();
        let r_point = point_bytes(ProjectivePoint::GENERATOR * r);
        let t_point = point_bytes(self.message + buyer * r);
        let u_point = point_bytes(ProjectivePoint::GENERATOR * a);
        let v_point = point_bytes(buyer * a - h * d);
        let points = [
            self.commitment(),
            point_bytes(buyer),
            r_point,
            t_point,
            u_point,
            v_point,
        ];
        let c = challenge(context, &points);
        Delivery {
            r_point,
            t_point,
            u_point,
            v_point,
            z_r: (a + c * r).to_bytes().into(),
            z_b: (d + c * self.blind).to_bytes().into(),
        }
    }
}

pub fn commitment_from_witness(message: &PointBytes, blind: &[u8; 32]) -> Option<PointBytes> {
    let message = point_from_bytes(message)?;
    let blind = scalar_from_bytes(blind)?;
    if blind == Scalar::ZERO {
        return None;
    }
    let commitment = message + h_generator() * blind;
    if commitment == ProjectivePoint::IDENTITY {
        return None;
    }
    Some(point_bytes(commitment))
}

pub fn content_key_from_message(message: &PointBytes) -> Option<[u8; 32]> {
    Some(content_key(point_from_bytes(message)?))
}

/// Canonical compressed point for the 65-byte key stored by a funded order.
pub fn compressed_public_key(uncompressed: &[u8; 65]) -> Option<PointBytes> {
    if uncompressed[0] != 4 {
        return None;
    }
    let encoded = k256::EncodedPoint::from_bytes(uncompressed).ok()?;
    let point = Option::<AffinePoint>::from(AffinePoint::from_encoded_point(&encoded))?;
    if point == AffinePoint::IDENTITY {
        return None;
    }
    Some(point_bytes(ProjectivePoint::from(point)))
}

/// Canonical order context, built from stored contract and funded-order state.
/// All fields have fixed lengths, so no caller-chosen delimiter can alias one
/// order with another.
pub fn order_context(
    network: &[u8; 32],
    contract: &[u8; 20],
    bundle: &[u8; 32],
    order_id: u64,
    nonce: u64,
    buyer_address: &[u8; 20],
    buyer_key: &PointBytes,
) -> [u8; 32] {
    let mut hash = Sha256::new();
    hash.update(ORDER_DOMAIN);
    hash.update(network);
    hash.update(contract);
    hash.update(bundle);
    hash.update(order_id.to_be_bytes());
    hash.update(nonce.to_be_bytes());
    hash.update(buyer_address);
    hash.update(buyer_key);
    hash.finalize().into()
}

pub fn verify(
    commitment: &PointBytes,
    buyer: &PointBytes,
    context: &[u8],
    delivery: &Delivery,
) -> bool {
    if context.is_empty() || context.len() > MAX_CONTEXT_BYTES {
        return false;
    }
    let Some(commitment_point) = point_from_bytes(commitment) else {
        return false;
    };
    let Some(buyer_point) = point_from_bytes(buyer) else {
        return false;
    };
    let Some(r_point) = point_from_bytes(&delivery.r_point) else {
        return false;
    };
    let Some(t_point) = point_from_bytes(&delivery.t_point) else {
        return false;
    };
    let Some(u_point) = point_from_bytes(&delivery.u_point) else {
        return false;
    };
    let Some(v_point) = point_from_bytes(&delivery.v_point) else {
        return false;
    };
    let Some(z_r) = scalar_from_bytes(&delivery.z_r) else {
        return false;
    };
    let Some(z_b) = scalar_from_bytes(&delivery.z_b) else {
        return false;
    };
    let c = challenge(
        context,
        &[
            *commitment,
            *buyer,
            delivery.r_point,
            delivery.t_point,
            delivery.u_point,
            delivery.v_point,
        ],
    );
    ProjectivePoint::GENERATOR * z_r == u_point + r_point * c
        && buyer_point * z_r - h_generator() * z_b == v_point + (t_point - commitment_point) * c
}

#[cfg(feature = "prover")]
fn nonzero_scalar() -> Scalar {
    loop {
        let scalar = Scalar::random(&mut OsRng);
        if scalar != Scalar::ZERO {
            return scalar;
        }
    }
}

/// The caller supplies the registered order's commitment, buyer key and exact
/// context. This checks the proof and secret-key ownership before recovery.
pub fn buyer_content_key(
    secret: &[u8; 32],
    commitment: &PointBytes,
    buyer: &PointBytes,
    context: &[u8],
    delivery: &Delivery,
) -> Option<[u8; 32]> {
    let secret = scalar_from_bytes(secret)?;
    if secret == Scalar::ZERO {
        return None;
    }
    if point_bytes(ProjectivePoint::GENERATOR * secret) != *buyer
        || !verify(commitment, buyer, context, delivery)
    {
        return None;
    }
    let r_point = point_from_bytes(&delivery.r_point)?;
    let t_point = point_from_bytes(&delivery.t_point)?;
    let message = t_point - r_point * secret;
    if message == ProjectivePoint::IDENTITY {
        return None;
    }
    Some(content_key(message))
}

fn content_key(message: ProjectivePoint) -> [u8; 32] {
    let mut hash = Sha256::new();
    hash.update(KEY_DOMAIN);
    hash.update(point_bytes(message));
    hash.finalize().into()
}

#[cfg(test)]
mod tests {
    extern crate std;
    use super::*;
    use k256::elliptic_curve::hash2curve::{ExpandMsgXmd, GroupDigest};
    use k256::Secp256k1;

    fn scalar(n: u64) -> Scalar {
        Scalar::from(n)
    }

    #[test]
    fn the_generator_pin_uses_hash_to_curve() {
        let derived = Secp256k1::hash_from_bytes::<ExpandMsgXmd<Sha256>>(
            &[b"Flips on Demand independent commitment generator v1"],
            &[H_DST],
        )
        .unwrap();
        assert_eq!(point_bytes(derived), H_BYTES);
        assert_ne!(derived, ProjectivePoint::GENERATOR);
    }

    #[test]
    fn buyer_recovers_the_registered_key_and_context_is_bound() {
        let seller = SellerKey {
            message: ProjectivePoint::GENERATOR * scalar(13),
            blind: scalar(23),
        };
        let buyer_secret = scalar(17);
        let buyer = point_bytes(ProjectivePoint::GENERATOR * buyer_secret);
        let context = b"synthetic network/contract/bundle/order/nonce/buyer";
        let delivery = seller.deliver_with_randomness(
            ProjectivePoint::GENERATOR * buyer_secret,
            context,
            scalar(19),
            scalar(29),
            scalar(31),
        );
        let commitment = seller.commitment();
        assert!(verify(&commitment, &buyer, context, &delivery));
        assert_eq!(
            buyer_content_key(
                &buyer_secret.to_bytes().into(),
                &commitment,
                &buyer,
                context,
                &delivery
            ),
            Some(seller.content_key())
        );
        assert!(!verify(&commitment, &buyer, b"another order", &delivery));

        let another_buyer = point_bytes(ProjectivePoint::GENERATOR * scalar(37));
        assert!(!verify(&commitment, &another_buyer, context, &delivery));
        let wrong_commitment = point_bytes(ProjectivePoint::GENERATOR * scalar(41));
        assert!(!verify(&wrong_commitment, &buyer, context, &delivery));
        let mut tampered = delivery;
        tampered.z_r[0] ^= 1;
        assert!(!verify(&commitment, &buyer, context, &tampered));
        tampered = delivery;
        tampered.t_point = point_bytes(ProjectivePoint::GENERATOR * scalar(41));
        assert!(!verify(&commitment, &buyer, context, &tampered));
    }

    #[test]
    fn malformed_points_and_scalars_are_refused() {
        let seller = SellerKey::generate();
        let buyer_secret = Scalar::random(&mut OsRng);
        let buyer = point_bytes(ProjectivePoint::GENERATOR * buyer_secret);
        let mut delivery = seller.deliver(&buyer, b"synthetic order").unwrap();
        assert!(verify(
            &seller.commitment(),
            &buyer,
            b"synthetic order",
            &delivery
        ));
        delivery.t_point = [0; 33];
        assert!(!verify(
            &seller.commitment(),
            &buyer,
            b"synthetic order",
            &delivery
        ));
        assert!(seller.deliver(&[0; 33], b"synthetic order").is_none());
    }

    #[test]
    fn buyer_rejects_an_identity_content_point() {
        let seller = SellerKey {
            message: ProjectivePoint::IDENTITY,
            blind: scalar(23),
        };
        let buyer_secret = scalar(17);
        let buyer = point_bytes(ProjectivePoint::GENERATOR * buyer_secret);
        let context = b"synthetic order";
        let delivery = seller.deliver_with_randomness(
            ProjectivePoint::GENERATOR * buyer_secret,
            context,
            scalar(7),
            scalar(11),
            scalar(13),
        );
        let commitment = seller.commitment();
        assert!(verify(&commitment, &buyer, context, &delivery));
        assert_eq!(
            buyer_content_key(
                &buyer_secret.to_bytes().into(),
                &commitment,
                &buyer,
                context,
                &delivery
            ),
            None
        );
        assert_eq!(
            buyer_content_key(&[0; 32], &commitment, &buyer, context, &delivery),
            None
        );
        let wrong_buyer = point_bytes(ProjectivePoint::GENERATOR * scalar(41));
        assert_eq!(
            buyer_content_key(
                &buyer_secret.to_bytes().into(),
                &commitment,
                &wrong_buyer,
                context,
                &delivery
            ),
            None
        );
    }

    #[test]
    fn content_witness_and_order_context_are_bound() {
        let seller = SellerKey {
            message: ProjectivePoint::GENERATOR * scalar(13),
            blind: scalar(23),
        };
        let (message, blind) = seller.witness_material();
        assert_eq!(
            commitment_from_witness(&message, &blind),
            Some(seller.commitment())
        );
        assert_eq!(
            content_key_from_message(&message),
            Some(seller.content_key())
        );
        assert_eq!(commitment_from_witness(&message, &[0; 32]), None);
        assert_eq!(content_key_from_message(&[0; 33]), None);
        assert_eq!(
            SellerKey::from_witness(&message, &blind)
                .unwrap()
                .commitment(),
            seller.commitment()
        );

        let buyer = point_bytes(ProjectivePoint::GENERATOR * scalar(17));
        let base = order_context(&[1; 32], &[2; 20], &[3; 32], 7, 9, &[4; 20], &buyer);
        assert_ne!(
            base,
            order_context(&[1; 32], &[2; 20], &[3; 32], 8, 9, &[4; 20], &buyer)
        );
        assert_ne!(
            base,
            order_context(&[1; 32], &[2; 20], &[3; 32], 7, 10, &[4; 20], &buyer)
        );
        assert_ne!(
            base,
            order_context(&[1; 32], &[2; 20], &[3; 32], 7, 9, &[5; 20], &buyer)
        );
    }
}
