import test from 'node:test'
import assert from 'node:assert/strict'
import {
  atoms,
  idna,
  suggestion,
  marketActions,
  initialTradeLimit,
  actionProgress,
  demoStatus,
  demoPurchasedPreview,
  purchasedTrades,
  purchasedFrames,
  terms,
  ATOMS,
} from './standalone_v8_model.mjs'
test('v9 starts at 500 iDNA and preview scenarios use coherent quotes and deposits', () => {
  const m = demoStatus('ready', 9).market
  assert.equal(m.buyQuote, (500n * ATOMS).toString())
  assert.equal(m.sellQuote, '409090909090909090909')
  assert.equal(marketActions(m).canBuy, true)
  assert.equal(marketActions(m).canSell, true)
  assert.equal(marketActions(demoStatus('demand', 9).market).sellNow, true)
  assert.equal(demoStatus('pool', 9).market.sellQuote, (500n * ATOMS).toString())
  assert.equal(suggestion(demoStatus('waiting', 9).market), (750n * ATOMS).toString())
  assert.equal(demoStatus('ready', 8).market.buyQuote, '11111111111111111112')
})
test('seller trade limit starts high enough for sale capacity, including reserved deposit', () => {
  const m = demoStatus('ready', 9).market
  const quoted = terms(m)
  assert.equal(initialTradeLimit(m, false), quoted.capacity)
  assert.ok(BigInt(initialTradeLimit(m, false)) > BigInt(quoted.maximum))
  assert.equal(initialTradeLimit(m, true), quoted.maximum)
})
test('transaction progress gives a state and an actionable next step', () => {
  assert.deepEqual(actionProgress({ operation: 'queueSell', attempted: true }), {
    state: 'Needs reconciliation',
    next: 'Next: check the chain result before another action.',
  })
  assert.equal(actionProgress({ operation: 'queueSell', submission: { included: true, success: false } }).state, 'Failed on chain')
  assert.equal(actionProgress({ operation: 'queueSell', attempted: true, submission: { transactionHash: '0xabc', included: false } }).state, 'Sent, awaiting a block')
  assert.equal(actionProgress({ operation: 'depositStake', submission: { included: true, success: true } }).next, 'Next: return to Market to place an order.')
})
test('iDNA conversion preserves exact base units and rejects ambiguous amounts', () => {
  for (const amount of ['0', '17', '0.000000000000000001', '123456789012345678.123456789012345678'])
    assert.equal(idna(atoms(amount)), amount)
  for (const amount of ['-1', '1e3', '1.0000000000000000001', 'NaN', '01'])
    assert.throws(() => atoms(amount))
})
test('first-use deposit covers current curve capacity and outstanding exposure', () => {
  const m = demoStatus().market
  assert.equal(suggestion(m), (17n * ATOMS).toString())
  m.account = {
    balance: (17n * ATOMS).toString(),
    target: (17n * ATOMS).toString(),
    reserved: (10n * ATOMS).toString(),
    needsTopup: false,
  }
  assert.equal(suggestion(m), (27n * ATOMS).toString())
  assert.equal(marketActions(m).canSell, false)
  m.account = {
    balance: (100n * ATOMS).toString(),
    target: (5n * ATOMS).toString(),
    reserved: '0',
    needsTopup: false,
  }
  assert.equal(marketActions(m).canBuy, false)
})
test('refill freezes new trades, including with spare collateral', () => {
  const m = demoStatus('ready').market
  assert.equal(marketActions(m).canBuy, true)
  m.account.needsTopup = true
  assert.equal(marketActions(m).canBuy, false)
  assert.equal(marketActions(m).canSell, false)
})
test('queue labels respect other orders, self-trades and distinct curve quotes', () => {
  const m = demoStatus('ready').market
  m.sells = [
    {
      id: '1',
      seller: 'another-seller',
      minimum: (10n * ATOMS).toString(),
      capacity: (20n * ATOMS).toString(),
      expires: 2000,
      state: 1,
    },
  ]
  assert.equal(
    marketActions(m).buyNow,
    false,
    'minimum above seller proceeds cannot be satisfied by the higher buyer quote'
  )
  m.sells[0].minimum = (8n * ATOMS).toString()
  assert.equal(marketActions(m).buyNow, true)
  m.sells[0].seller = m.identity
  assert.equal(marketActions(m).buyNow, false)
  m.sells[0].seller = 'another-seller'
  m.buys = [{ buyer: 'earlier-buyer', maximum: (20n * ATOMS).toString(), expires: 2000, state: 1 }]
  assert.equal(marketActions(m).buyNow, false)
  m.sells = []
  assert.equal(marketActions(m).sellNow, true)
  m.buys[0].buyer = m.identity
  assert.equal(marketActions(m).sellNow, false)
})
test('pool readiness and fixed proceeds on acquired packages are explicit', () => {
  const m = demoStatus('ready').market
  assert.equal(marketActions(m).sellLabel, 'Join sell queue')
  m.poolCanAcquire = true
  assert.equal(marketActions(m).sellLabel, 'Sell to pool')
  m.sells = [
    {
      id: '1',
      seller: 'seller',
      minimum: (10n * ATOMS).toString(),
      proceeds: (12n * ATOMS).toString(),
      capacity: (20n * ATOMS).toString(),
      expires: 2000,
      state: 2,
    },
  ]
  assert.equal(marketActions(m).buyNow, true, 'acquired sales retain agreed proceeds')
  m.sells[0].expires = 1000
  assert.equal(marketActions(m).buyNow, false)
})
test('completed purchases belong to the buyer and keep both original sequences', () => {
  const m = demoStatus('purchased', 9).market
  m.trades.push({ id: '8', buyer: 'another-buyer', seller: m.identity, state: 5, price: '1' })
  m.trades.push({ id: '9', buyer: m.identity, state: 3, price: '1' })
  assert.deepEqual(purchasedTrades(m).map((trade) => trade.id), ['7'])
  const preview = demoPurchasedPreview('7')
  assert.equal(preview.count, preview.flips.length)
  assert.equal(preview.flips.length, 2)
  const first = purchasedFrames(preview.flips[0])
  assert.deepEqual(first[0], preview.flips[0].images)
  assert.deepEqual(first[1], [
    preview.flips[0].images[1],
    preview.flips[0].images[0],
    preview.flips[0].images[2],
    preview.flips[0].images[3],
  ])
  assert.throws(() => purchasedFrames({ images: preview.flips[0].images, orders: [[0, 0, 1, 2], [0, 1, 2, 3]] }))
  assert.throws(() => purchasedFrames({ images: ['https://example.org/a.png', ...preview.flips[0].images.slice(1)], orders: preview.flips[0].orders }))
  assert.equal(terms(m).maximum, (525n * ATOMS).toString())
})
