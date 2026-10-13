// Local FAQ content. The view renders each answer as text.

export interface FaqEntry {
  question: string;
  answer: string[];
}

export interface FaqSection {
  title: string;
  entries: FaqEntry[];
}

export const FAQ: FaqSection[] = [
  {
    title: "What this is",
    entries: [
      {
        question: "What does this page show?",
        answer: [
          "An experimental market for copies of the original keys to historical Idena flips. A sale unit covers one author and one epoch. Keys can be copied and sold again.",
          "The market view reads a file you choose. Its listings, prices, block number, and status are claims from that file, not independently verified chain state.",
        ],
      },
    ],
  },
  {
    title: "Buying and selling",
    entries: [
      {
        question: "Can this page move my iDNA?",
        answer: [
          "Importing a snapshot never signs or sends a transaction. Earlier market views can save an action request; the separate command-line tool must recheck the contract and prepare any signed transaction.",
          "The liquidity-pool view is a research preview. Its quote is informational, and its contract and dispute path are not qualified for funded use.",
        ],
      },
      {
        question: "How are flip keys delivered?",
        answer: [
          "A seller may send keys privately. A buyer can verify the original blobs and confirm receipt. If the trade is disputed, recovery can require a costly proof or publication of keys, depending on the contract version.",
          "A private message is not itself proof that the buyer received usable keys. A seller who reveals usable data before settlement is included can lose the chance to be paid.",
        ],
      },
    ],
  },
  {
    title: "Sessions and gated access",
    entries: [
      {
        question: "Does using flips prove I am a unique person?",
        answer: [
          "No. Completing an independently scheduled session only supports the specific result that its provider and utility can verify. It does not establish unique identity or reproduce Idena's coordinated validation.",
        ],
      },
      {
        question: "Which services does a session contact?",
        answer: [
          "A session contacts the gateway and, when configured, a separate utility. Those services receive the data needed for enrollment, answers, scoring, or a grant. Check the imported connection file and its endpoints before signing a challenge.",
          "The provider signs a result, but it may withhold that result or stop serving the session. The utility applies its own policy and can still refuse a grant.",
        ],
      },
    ],
  },
  {
    title: "Costs",
    entries: [
      {
        question: "What could a trade cost?",
        answer: [
          "Network fees, escrowed payment, bonds, and proof costs depend on the contract version and current chain rate. Historical measurements in this repository are not a current fee quote.",
        ],
      },
    ],
  },
  {
    title: "Security limits",
    entries: [
      {
        question: "What should I verify myself?",
        answer: [
          "Confirm the contract address and code, current chain state, fees, refund rules, available blobs, and the provenance of any imported file. A recent timestamp is not proof that a snapshot came from a trusted node.",
          "A displayed provider result can be unverified; check its verification status before relying on it. Public flip identifiers and on-chain transactions can link addresses and activity.",
        ],
      },
    ],
  },
];
