export const ATOMS = 10n ** 18n
export function atoms(value) {
  if (!/^(0|[1-9]\d*)(\.\d{1,18})?$/.test(value))
    throw Error('Enter a non-negative iDNA amount with up to 18 decimals.')
  const [a, b = ''] = value.split('.')
  return BigInt(a) * ATOMS + BigInt(b.padEnd(18, '0'))
}
export function idna(value) {
  const n = BigInt(value || '0')
  const sign = n < 0n ? '-' : ''
  const x = n < 0n ? -n : n
  return (
    sign +
    (x / ATOMS).toString() +
    (x % ATOMS ? '.' + (x % ATOMS).toString().padStart(18, '0').replace(/0+$/, '') : '')
  )
}
export function approximate(value) {
  const s = idna(value)
  const [a, b = ''] = s.split('.')
  return a + (b ? '.' + b.slice(0, 4) : '')
}
export function suggestion(m) {
  const need = BigInt(m.account.reserved) + (BigInt(m.buyQuote) * 150n + 99n) / 100n
  const target = BigInt(m.account.target)
  const desired = need > target ? need : target
  return (((desired + ATOMS - 1n) / ATOMS) * ATOMS).toString()
}
export function terms(m) {
  return {
    maximum: ((BigInt(m.buyQuote) * 105n + 99n) / 100n).toString(),
    minimum: ((BigInt(m.sellQuote) * 95n) / 100n).toString(),
    capacity: ((BigInt(m.buyQuote) * 150n + 99n) / 100n).toString(),
  }
}
export function initialTradeLimit(m, buyerOnly = false) {
  const t = terms(m)
  return buyerOnly || BigInt(t.maximum) >= BigInt(t.capacity) ? t.maximum : t.capacity
}
export function actionProgress(action) {
  if (action.submission?.included)
    return action.submission.success
      ? {
          state: 'Confirmed on chain',
          next: action.operation === 'depositStake'
            ? 'Next: return to Market to place an order.'
            : 'Next: review the updated order or trade above.',
        }
      : { state: 'Failed on chain', next: 'Next: review the chain note and retry the action only if still needed.' }
  if (action.attempted)
    return action.submission?.transactionHash
      ? { state: 'Sent, awaiting a block', next: 'Next: wait for inclusion; use Check chain result if it takes more than a few blocks.' }
      : { state: 'Needs reconciliation', next: 'Next: check the chain result before another action.' }
  return { state: 'Prepared, unsigned', next: 'No transaction was sent. Start the action again if still needed.' }
}
export function eligibleSells(m) {
  return m.sells.filter((s) => (s.state === 1 || s.state === 2) && s.expires > m.height)
}
export function eligibleBuys(m) {
  return m.buys.filter((b) => b.state === 1 && b.expires > m.height)
}
export function purchasedTrades(m) {
  return m.trades.filter((t) => t.buyer === m.identity && t.state === 5)
}
export function purchasedFrames(flip) {
  const images = flip?.images
  const orders = flip?.orders
  if (
    !Array.isArray(images) ||
    images.length !== 4 ||
    !images.every((url) =>
      typeof url === 'string' && /^data:image\/(?:png|jpeg);base64,[A-Za-z0-9+/]+={0,2}$/.test(url)
    ) ||
    !Array.isArray(orders) ||
    orders.length !== 2 ||
    !orders.every(
      (order) =>
        Array.isArray(order) &&
        order.length === 4 &&
        order.every((index) => Number.isInteger(index) && index >= 0 && index <= 3) &&
        new Set(order).size === 4
    )
  )
    throw Error('Purchased flip images or sequences are unavailable.')
  return orders.map((order) => order.map((index) => images[index]))
}
export function marketActions(m) {
  const t = terms(m),
    balance = BigInt(m.account.balance),
    target = BigInt(m.account.target),
    free = (balance < target ? balance : target) - BigInt(m.account.reserved),
    healthy = !m.account.needsTopup && balance >= target
  const sellers = eligibleSells(m).filter(
    (s) =>
      BigInt(s.minimum) <= BigInt(s.state === 2 ? s.proceeds : m.sellQuote) &&
      BigInt(s.capacity) >= BigInt(m.buyQuote)
  )
  const buyers = eligibleBuys(m).filter((b) => BigInt(b.maximum) >= BigInt(m.buyQuote))
  const buyNow = sellers.some((s) => s.seller !== m.identity) && buyers.length === 0
  const sellNow = buyers.some((b) => b.buyer !== m.identity) && sellers.length === 0
  return {
    buyNow,
    sellNow,
    buyLabel: buyNow ? 'Buy now' : 'Join buy queue',
    sellLabel: sellNow
      ? 'Sell now'
      : m.poolCanAcquire && eligibleSells(m).filter((s) => s.state === 1).length === 0
      ? 'Sell to pool'
      : 'Join sell queue',
    canBuy: healthy && free >= BigInt(t.maximum),
    canSell: healthy && free >= BigInt(t.capacity),
    free: free.toString(),
    ...t,
  }
}
export function demoStatus(scenario = 'waiting', version = 8) {
  const m = {
    version,
    identity: 'example-owner',
    height: 1000,
    reserve: '0',
    escrow: '0',
    stakes: '0',
    pendingEarnings: '0',
    inventory: '0',
    buyQuote: '11111111111111111112',
    sellQuote: '9090909090909090909',
    poolCanAcquire: false,
    account: { balance: '0', reserved: '0', target: '0', needsTopup: false },
    sells: [],
    buys: [],
    trades: [],
    bundles: [],
    priceHistory: [],
  }
  const p = {
    id: 'example-package',
    epoch: 233,
    cids: Array(5).fill('example'),
    retained: 5,
    status: 'waiting',
    reason: 'Waiting for validation. Keep your node and this app open.',
    vault: true,
    witness: true,
    session: false,
    registered: false,
  }
  if (scenario !== 'waiting') {
    m.account = {
      balance: '34000000000000000000',
      reserved: '0',
      target: '34000000000000000000',
      needsTopup: false,
    }
    p.status = 'ready'
    p.reason = 'Registered package. Your encrypted key vault is available.'
    p.session = true
    p.registered = version === 8
    if (version === 9) p.reason = "Validation verified. Ready to sell with your deposit."
    const examplePrices = version === 9 ? ['450', '480', '500'] : ['9', '10', '12']
    m.priceHistory = examplePrices.map((price, i) => ({
      trade: String(i + 5), height: 960 + i * 10, price: (BigInt(price) * ATOMS).toString(),
    }))
  }
  if (scenario === 'demand') {
    m.buys = [
      { id: '1', buyer: 'example-buyer', maximum: '13000000000000000000', expires: 5320, state: 1 },
    ]
  }
  if (scenario === 'pool') {
    m.reserve = '100000000000000000000'
    m.poolCanAcquire = true
  }
  if (scenario === 'refill') {
    m.account.balance = '5000000000000000000'
    m.account.needsTopup = true
  }
  if (scenario === 'purchased') {
    m.trades = [{ id: '7', buyer: m.identity, seller: 'example-seller', state: 5,
      price: version === 9 ? (500n * ATOMS).toString() : (12n * ATOMS).toString() }]
  }
  if (scenario === 'buyer') {
    m.sells = [{ id: '2', seller: 'example-seller', minimum: (475n * ATOMS).toString(),
      capacity: (750n * ATOMS).toString(), expires: 5320, state: 1 }]
  }
  if (version === 9) {
    if (scenario !== 'waiting') {
      m.account.target = (1500n * ATOMS).toString()
      m.account.balance = ((scenario === 'refill' ? 250n : 1500n) * ATOMS).toString()
    }
    if (scenario === 'demand') m.buys[0].maximum = (600n * ATOMS).toString()
    if (scenario === 'pool') m.reserve = (1000n * ATOMS).toString()
    if (scenario === 'buyer') {
      m.account.target = (750n * ATOMS).toString()
      m.account.balance = (750n * ATOMS).toString()
    }
    const numerator = (BigInt(m.reserve) + 4500n * ATOMS) * 100n
    const units = BigInt(m.inventory) + 1000n
    m.buyQuote = ((numerator + units - 101n) / (units - 100n)).toString()
    m.sellQuote = (numerator / (units + 100n)).toString()
  }
  if (scenario !== 'waiting') m.stakes = m.account.balance
  if (scenario === 'demand') m.escrow = m.buys[0].maximum
  return {
    market: m,
    buyerOnly: scenario === 'buyer',
    epoch: 233,
    wallet: version === 9 ? '5000' : '100',
    packages: scenario === 'buyer' ? [] : [p],
    canSign: false,
    automatic: false,
    message: 'Example data only. Nothing is connected.',
    capture: 'Preview only — no capture process is running.',
  }
}
const demoImages = [
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAADAAAAAkCAIAAABAJy5dAAAAVklEQVR42u3UQQkAIBAAwYtkCcG/KUxiQBv4NIUB9CEId4cubIB5rbTRXSWAAAEyAYWY18xAW42aSc41OiZAgAB9/yFODQgQIECAAN2CUi2uAgToOdAEznaO9JB4jgIAAAAASUVORK5CYII=',
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAADAAAAAkCAIAAABAJy5dAAAAV0lEQVR42u3UsQ0AEBgF4X8kS0j0pjCJsaxiCrUBaATxIpfcAF911mqRygABAqQDcj6OPQNNNTdMtqk5bgIECJAESO5DnBoQIECAAAFaAIWcpAIE6DtQB6oKXy7Y0sQBAAAAAElFTkSuQmCC',
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAADAAAAAkCAIAAABAJy5dAAAAVklEQVR42u3UQQkAIBAF0Y1kCcG7KUxiSg92MIAB3NsifmRgArzT2JhLKgMECJA4KOV69gzkaoImu6GJmAABAqQLkvsQpwYECBAgQIB8UOlNKkCAvgNtlqHpuaPNkbYAAAAASUVORK5CYII=',
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAADAAAAAkCAIAAABAJy5dAAAAVUlEQVR42u3UMQ0AIBDF0C8JEyTsqEAJBjGACWYEwMAEd6RJBbypGq2bSoAAAfIICjGvPQNtNecmXdMcmgABAuQMZO5DnBoQIECAAAFSqsVUgAB9B5pFTshIAjir1QAAAABJRU5ErkJggg==',
]
export function demoPurchasedPreview(id) {
  if (id !== '7') throw Error('No example purchase found.')
  return {
    trade: id,
    epoch: 233,
    count: 2,
    flips: [
      { cid: 'example-flip-1', images: demoImages, orders: [[0, 1, 2, 3], [1, 0, 2, 3]] },
      { cid: 'example-flip-2', images: demoImages, orders: [[3, 2, 1, 0], [2, 3, 1, 0]] },
    ],
  }
}
