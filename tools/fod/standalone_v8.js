import {
  atoms,
  idna,
  approximate,
  suggestion,
  marketActions,
  initialTradeLimit,
  actionProgress,
  eligibleBuys,
  eligibleSells,
  purchasedTrades,
  purchasedFrames,
  demoStatus,
  demoPurchasedPreview,
} from '/model.mjs'
const $ = (id) => document.getElementById(id)
let token = '',
  preview = false,
  ownNodeMode = false,
  protocolVersion = 8,
  status = null,
  selected = null,
  packageToProtect = '',
  limitsInitialized = false,
  limitsMode = '',
  pauseReason = '',
  lastAutomatic = null,
  busy = false,
  tab = 'flips',
  buyerFirstRender = true,
  purchasedPreview = null,
  purchasedIndex = 0
const labels = {
  collecting: 'Collecting',
  waiting: 'Waiting for validation',
  protect: 'Protect keys',
  prepare: 'Needs proof',
  missing: 'Missing data',
  unqualified: 'Not eligible to sell',
  ready: 'Ready to sell',
  queued: 'Sale in progress',
  sold: 'Sold',
}
const operationLabels = {
  depositStake: 'Security deposit',
  withdrawStake: 'Finish trading & withdraw deposit',
  queueBuy: 'Buy order',
  queueSell: 'Sell order',
  cancelBuy: 'Cancel buy order',
  cancelSell: 'Cancel sell order',
  registerBundle: 'Register package',
  matchNext: 'Match queued orders',
  acquire: 'Reserve a pool sale',
  ready: 'Confirm encrypted flips retained',
  deliver: 'Send private encrypted keys',
  accept: 'Accept verified delivery',
  challenge: 'Dispute delivery',
  expireTrade: 'Settle expired trade',
  proveDelivery: 'Submit delivery proof',
  addReserve: 'Contribute pool funds',
}
function message(text) {
  $('notice').textContent = text
}
function el(tag, text, className) {
  const x = document.createElement(tag)
  if (text !== undefined) x.textContent = text
  if (className) x.className = className
  return x
}
function button(text, fn, disabled = false) {
  const b = el('button', text)
  b.type = 'button'
  b.disabled = disabled
  b.addEventListener('click', () => perform(fn))
  return b
}
async function api(path, body) {
  if (preview) throw Error('Preview only. No transaction or private file is connected.')
  const headers = { 'X-FOD-Token': token }
  if (body && !(body instanceof FormData)) headers['Content-Type'] = 'application/json'
  const r = await fetch(path, {
    method: body ? 'POST' : 'GET',
    headers,
    body: body instanceof FormData ? body : body ? JSON.stringify(body) : undefined,
    cache: 'no-store',
  })
  if (!r.ok) throw Error((await r.text()).slice(0, 350))
  return r.json()
}
async function perform(fn, errorId) {
  if (busy) return
  busy = true
  if (errorId) {
    $(errorId).textContent = ''
    $(errorId).hidden = true
  }
  message('Working locally…')
  try {
    await fn()
  } catch (e) {
    message(e.message)
    if (errorId) {
      $(errorId).textContent = e.message
      $(errorId).hidden = false
    }
  } finally {
    busy = false
  }
}
function navigate(next) {
  tab = next
  for (const t of ['flips', 'market', 'activity', 'community']) {
    $('tab-' + t).hidden = t !== next
    const b = document.querySelector('[data-tab="' + t + '"]')
    if (t === next) b.setAttribute('aria-current', 'page')
    else b.removeAttribute('aria-current')
  }
  $('page-title').textContent = {
    flips: status?.buyerOnly ? 'My purchases' : 'My flips',
    market: 'Market', activity: 'Activity', community: 'Community',
  }[next]
  if (next === 'community' && !$('social-frame').firstChild) {
    const frame = el('iframe')
    frame.title = 'idena.social community'
    frame.src = 'https://idena.social/'
    frame.loading = 'lazy'
    frame.referrerPolicy = 'no-referrer'
    frame.setAttribute('sandbox', 'allow-scripts allow-same-origin')
    $('social-frame').append(frame)
  } else if (next !== 'community') {
    $('social-frame').replaceChildren()
  }
  renderNextStep()
}
function renderNextStep() {
  if (!status) return
  const m = status.market,
    packages = status.packages,
    balance = BigInt(m.account.balance),
    a = marketActions(m)
  $('next-step').hidden = tab === 'community'
  if (status.buyerOnly) {
    $('next-step').textContent = tab === 'market'
      ? balance === 0n || m.account.needsTopup || !a.canBuy
        ? 'Set up your reusable security deposit before placing a buy order.'
        : !status.automatic
        ? 'Set your local spending limits, then place a buy order.'
        : 'Buy a set now or leave your funded order in the queue.'
      : tab === 'activity'
      ? 'Track your orders, delivery deadlines and confirmed transactions here.'
      : purchasedTrades(m).length
      ? 'Your completed purchases are ready to view below.'
      : 'Your purchases will appear here after delivery is verified.'
    return
  }
  if (tab === 'market') {
    $('next-step').textContent = balance === 0n || m.account.needsTopup || (!a.canBuy && !a.canSell)
      ? 'Next: set or refill your security deposit before trading.'
      : !status.automatic
      ? 'Next: enable one-click trading, then choose Buy or Sell.'
      : 'Choose Buy or Sell. An unmatched order stays in the shared queue.'
  } else if (tab === 'activity') {
    $('next-step').textContent = 'Review open orders, delivery deadlines and confirmed transactions here.'
  } else if (packages.some((p) => p.status === 'protect')) {
    $('next-step').textContent = 'Next: protect the keys for your checked package.'
  } else if (packages.some((p) => p.status === 'prepare')) {
    $('next-step').textContent = 'Next: add the completed content proof in Activity.'
  } else if (purchasedTrades(m).length) {
    $('next-step').textContent = 'Your completed purchases are ready to view below.'
  } else if (packages.some((p) => p.status === 'ready')) {
    $('next-step').textContent = 'Your package is ready. Set your security deposit, then open Market to sell.'
  } else if (packages.some((p) => p.status === 'waiting' || p.status === 'collecting')) {
    $('next-step').textContent = status.sellerClient
      ? 'Next: wait for the shared service to capture validation evidence.'
      : 'Next: keep your node and this app running through validation.'
  } else {
    $('next-step').textContent = 'Collect your own flips here, or open Market to buy a set.'
  }
}
function shortIdentity(value) {
  return value.length > 18 ? value.slice(0, 10) + '…' + value.slice(-6) : value
}
function renderPrices(m) {
  const box = $('price-chart'),
    points = (m.priceHistory || []).filter((p) => Number.isFinite(Number(idna(p.price))))
  box.replaceChildren()
  $('price-range').textContent = points.length
    ? points.length + ' completed trade' + (points.length === 1 ? '' : 's')
    : ''
  if (!points.length) {
    box.append(el('p', 'No completed trade prices in the recent snapshot.', 'small'))
    return
  }
  const values = points.map((p) => Number(idna(p.price))),
    low = Math.min(...values),
    high = Math.max(...values),
    width = 640,
    left = 24,
    right = 616,
    top = 18,
    bottom = 150,
    ns = 'http://www.w3.org/2000/svg',
    svg = document.createElementNS(ns, 'svg')
  svg.setAttribute('viewBox', '0 0 ' + width + ' 170')
  svg.setAttribute('role', 'img')
  svg.setAttribute('aria-label', 'Completed trade prices from ' + approximate(points[0].price) + ' to ' + approximate(points.at(-1).price) + ' iDNA')
  const coordinates = values.map((value, i) => ({
    x: points.length === 1 ? width / 2 : left + (right - left) * i / (points.length - 1),
    y: high === low ? (top + bottom) / 2 : bottom - (bottom - top) * (value - low) / (high - low),
  }))
  const line = document.createElementNS(ns, 'polyline')
  line.setAttribute('points', coordinates.map((p) => p.x + ',' + p.y).join(' '))
  line.setAttribute('fill', 'none')
  line.setAttribute('stroke', '#176652')
  line.setAttribute('stroke-width', '3')
  svg.append(line)
  coordinates.forEach((p, i) => {
    const circle = document.createElementNS(ns, 'circle')
    circle.setAttribute('cx', p.x)
    circle.setAttribute('cy', p.y)
    circle.setAttribute('r', '4')
    circle.setAttribute('fill', '#176652')
    const title = document.createElementNS(ns, 'title')
    title.textContent = 'Trade #' + points[i].trade + ' · block ' + points[i].height + ' · ' + idna(points[i].price) + ' iDNA'
    circle.append(title)
    svg.append(circle)
  })
  box.append(svg, el('p', 'Latest: ' + approximate(points.at(-1).price) + ' iDNA at block ' + points.at(-1).height + '.', 'small'))
}
function renderQueues(m) {
  const sells = eligibleSells(m),
    buys = eligibleBuys(m),
    sellBox = $('sell-offers'),
    buyBox = $('buy-demand')
  $('sell-queue').textContent = 'Open sell offers · ' + sells.length
  $('buy-queue').textContent = 'Open buy demand · ' + buys.length
  sellBox.replaceChildren()
  buyBox.replaceChildren()
  for (const s of sells) {
    const row = el('p', undefined, 'queue-item')
    row.append(
      el('strong', s.seller === m.identity ? 'Your offer' : 'Seller ' + shortIdentity(s.seller)),
      el('span', (s.state === 2 ? 'Pool reserved · agreed proceeds ' + approximate(s.proceeds) : 'Minimum ' + approximate(s.minimum)) + ' iDNA · capacity ' + approximate(s.capacity) + ' iDNA · until block ' + s.expires)
    )
    sellBox.append(row)
  }
  for (const b of buys) {
    const row = el('p', undefined, 'queue-item')
    row.append(
      el('strong', b.buyer === m.identity ? 'Your demand' : 'Buyer ' + shortIdentity(b.buyer)),
      el('span', 'Maximum ' + approximate(b.maximum) + ' iDNA · until block ' + b.expires)
    )
    buyBox.append(row)
  }
  if (!sells.length) sellBox.append(el('p', 'No open sell offers.', 'small'))
  if (!buys.length) buyBox.append(el('p', 'No open buy demand.', 'small'))
}
function render() {
  if (!status) return
  const m = status.market,
    a = marketActions(m)
  if (status.buyerOnly && buyerFirstRender) {
    tab = 'market'
    buyerFirstRender = false
  }
  document.body.classList.toggle('buyer-only', status.buyerOnly === true)
  $('pairing').hidden = true
  $('workspace').hidden = false
  $('registration-form').closest('details').hidden = status.market.version === 9
  $('connection').textContent = preview ? 'Preview · Example data' : status.buyerOnly
    ? 'Buyer wallet · v' + status.market.version : status.sellerClient
      ? 'Seller via shared node · v' + status.market.version : 'Connected to your node · v' + status.market.version
  document.querySelector('[data-tab="flips"]').textContent = status.buyerOnly ? 'My purchases' : 'My flips'
  $('package-heading').hidden = status.buyerOnly
  $('package-copy').hidden = status.buyerOnly
  $('packages').hidden = status.buyerOnly
  $('package-import').hidden = status.buyerOnly
  $('capture').hidden = status.buyerOnly
  $('package-copy').textContent = status.sellerClient
    ? 'Collect & check reads your current-epoch flips from the shared relay. Older sets need saved validation evidence from your archive or the shared relay; add a session bundle below if needed. Original encrypted content stays on this computer.'
    : 'Collect & check reads your current-epoch flips from your node. Older sets need saved validation evidence; add a session bundle below. Original encrypted content stays on your computer.'
  $('sell-panel').hidden = status.buyerOnly
  $('sell-quote-box').hidden = status.buyerOnly
  $('relay-note').hidden = !status.buyerOnly && !status.sellerClient
  $('service-label').textContent = status.buyerOnly || status.sellerClient
    ? 'Chain and encrypted flips via your configured relay · Keys stay on this computer'
    : 'Local node · No hosted market service'
  $('unlock-copy').textContent = status.buyerOnly
    ? 'For eight hours, this app can place the buy orders you click and advance your purchases within the limits below. It accepts only after every original flip opens and the encrypted keys are saved. Keep the app open through delivery.'
    : 'For eight hours, this local app may place orders you click, advance the shared queues, deliver your encrypted keys and accept purchases only after every flip decrypts and is saved. Keep it open for automatic processing.'
  $('vault-copy').textContent = status.buyerOnly
    ? 'This password protects saved flip keys. It is separate from your wallet key. Use at least 12 characters and keep it for future access.'
    : 'This local password protects saved flip keys. It is separate from your identity key. Use at least 12 characters; sellers use their existing saved-set password.'
  $('epoch').textContent = 'Epoch ' + status.epoch
  $('identity').textContent = (status.buyerOnly ? 'Your buyer wallet' : status.sellerClient
    ? 'Your seller identity on this computer' : 'Your node identity') + ': ' + m.identity
  const target = BigInt(suggestion(m)),
    balance = BigInt(m.account.balance),
    reserved = BigInt(m.account.reserved)
  const needed = target > balance ? target - balance : 0n
  $('deposit').hidden = needed === 0n && !m.account.needsTopup
  $('deposit-title').textContent =
    balance === 0n
      ? 'First, set up your security deposit'
      : m.account.needsTopup
      ? 'Refill your deposit to resume trading'
      : 'Increase your trading capacity'
  $('deposit-amount').value = idna(needed)
  $('deposit-button').disabled = preview || !status.canSign || needed === 0n
  $('deposit-copy').textContent =
    balance === 0n
      ? 'Buyers and sellers keep a reusable deposit to cover open trades. Withdraw it after closing all orders, trades and disputes and finishing trading.'
      : 'Your deposit holds ' +
        approximate(m.account.balance) +
        ' iDNA, with ' +
        approximate(m.account.reserved) +
        ' reserved. Refill it to cover the current trade limit.'
  const box = $('packages')
  box.replaceChildren()
  for (const p of status.packages) {
    const row = el('article', undefined, 'package'),
      body = el('div'),
      title = el('h3', 'Epoch ' + p.epoch + ' · ' + p.cids.length + ' flips')
    title.append(el('span', labels[p.status] || p.status, 'tag'))
    body.append(
      title,
      el('p', p.reason),
      el(
        'p',
        p.retained +
          '/' +
          p.cids.length +
          ' encrypted flips checked' +
          (p.vault ? ' · Keys protected locally' : '') +
          (p.session ? ' · Session evidence verified' : ''),
        'small'
      )
    )
    row.append(body)
    if (p.status === 'protect')
      row.append(
        button(
          'Protect package',
          () => {
            packageToProtect = p.id
            $('protect').showModal()
            message('')
          },
          preview || !status.canSign
        )
      )
    else if (p.status === 'ready')
      row.append(
        button('Sell package', () => {
          navigate('market')
          $('sell-package').value = p.id
          message('')
        })
      )
    else if (p.status === 'prepare')
      row.append(
        button('Add proof', () => {
          navigate('activity')
          $('registration-form').closest('details').open = true
          $('registration-form').scrollIntoView({ behavior: 'smooth' })
          message('')
        })
      )
    box.append(row)
  }
  if (!status.packages.length)
    box.append(
      el(
        'p',
        status.sellerClient
          ? 'No packages found yet. Submit flips with your Idena app; the shared service can collect their public evidence.'
          : 'No packages found yet. Submit flips with your node, or add saved validation evidence.',
        'empty'
      )
    )
  const purchased = $('purchased')
  purchased.replaceChildren()
  for (const t of purchasedTrades(m)) {
    const row = el('article', undefined, 'package'),
      body = el('div'),
      actions = el('div', undefined, 'package-actions')
    body.append(
      el('h3', 'Purchase #' + t.id),
      el('p', 'Completed · ' + approximate(t.price) + ' iDNA', 'small')
    )
    actions.append(
      button('View flips', () => openPurchasedPreview(t.id)),
      button('Save encrypted set', () => downloadPurchase(t.id), preview)
    )
    row.append(body, actions)
    purchased.append(row)
  }
  if (!purchased.children.length)
    purchased.append(el('p', 'No completed purchases in the recent market snapshot.', 'small'))
  $('capture').textContent = status.capture
  $('buy-quote').textContent = '≈ ' + approximate(m.buyQuote) + ' iDNA'
  $('sell-quote').textContent = '≈ ' + approximate(m.sellQuote) + ' iDNA'
  $('buy-description').textContent = a.buyNow
    ? 'A compatible seller is waiting. Your maximum price includes 5% quote protection.'
    : 'No compatible seller is ready. Your funded order waits in the buy queue; the curve price is checked when matched.'
  $('sell-description').textContent = a.sellNow
    ? 'A buyer is waiting. Proceeds remain pending until delivery is verified.'
    : a.sellLabel === 'Sell to pool'
    ? 'The pool can reserve this purchase. Your proceeds remain pending until delivery to a buyer is verified.'
    : 'Your package waits for a compatible buyer or funded pool purchase. You can cancel a waiting order.'
  const old = $('sell-package').value
  $('sell-package').replaceChildren()
  for (const p of status.packages.filter((p) => p.status === 'ready')) {
    const option = el('option', 'Epoch ' + p.epoch + ' · ' + p.cids.length + ' flips')
    option.value = p.id
    $('sell-package').append(option)
  }
  if ([...$('sell-package').options].some((o) => o.value === old)) $('sell-package').value = old
  $('buy').textContent = a.buyLabel
  $('sell').textContent = a.sellLabel
  $('buy').disabled = preview || !status.automatic || !a.canBuy
  $('sell').disabled = preview || !status.automatic || !a.canSell || !$('sell-package').value
  $('sell-unavailable').textContent = preview
    ? 'Example only: selling is unavailable in this preview.'
    : !status.automatic
    ? 'Enable one-click trading first.'
    : !a.canSell
    ? 'Increase your available security deposit to cover this sale.'
    : !$('sell-package').value
    ? 'Collect and protect a sale-ready package first.'
    : ''
  $('sell-unavailable').hidden = !$('sell').disabled
  $('collateral-note').textContent =
    'Available security deposit: ' +
    approximate(a.free) +
    ' iDNA. Current buy limit: ' +
    approximate(a.maximum) +
    '; minimum seller proceeds: ' +
    approximate(a.minimum) +
    '. Network fees are additional.'
  $('reserve').textContent = approximate(m.reserve) + ' iDNA'
  $('escrow').textContent = approximate(m.escrow) + ' iDNA'
  $('stakes').textContent = approximate(m.stakes) + ' iDNA'
  $('pending-earnings').textContent = approximate(m.pendingEarnings) + ' iDNA'
  renderPrices(m)
  renderQueues(m)
  $('unlock-area').hidden = status.automatic
  $('unlocked').hidden = !status.automatic
  $('unlock-form').querySelector('button').disabled = preview || !status.canSign || balance === 0n
  const mode = m.identity + ':' + status.buyerOnly
  if (!limitsInitialized || limitsMode !== mode) {
    pauseReason = ''
    lastAutomatic = null
    $('unlock-form').elements.namedItem('perTrade').value = idna(initialTradeLimit(m, status.buyerOnly))
    $('unlock-form').elements.namedItem('purchaseBudget').value = idna(a.maximum)
    limitsInitialized = true
    limitsMode = mode
  }
  $('per-trade-help').textContent = status.buyerOnly
    ? 'Covers the maximum purchase price for one trade.'
    : 'The per-trade limit must cover a sale’s reserved security deposit. Current sale capacity: ' +
      approximate(a.capacity) + ' iDNA; set this limit at least that high to sell.'
  if (!status.automatic && status.pauseReason) pauseReason = status.pauseReason
  if (lastAutomatic === true && !status.automatic && !pauseReason)
    pauseReason = 'Authorization ended. It may have expired or stopped after a transaction issue.'
  $('automatic-status').textContent = status.automatic
    ? 'Automatic processing is on while this app stays open.'
    : pauseReason
    ? 'Paused: ' + pauseReason + ' Open orders may still have deadlines. Review Activity, then enter your saved-set password to resume with these limits.'
    : 'One-click trading is off. Enable it to process open trades. Restarting the app requires a new password entry.'
  $('unlock-form').querySelector('button').textContent = pauseReason
    ? 'Resume within these limits' : 'Enable within these limits'
  lastAutomatic = status.automatic
  $('budget').textContent = status.automatic
    ? 'Automatic processing enabled · ' +
      approximate(status.feeBudget) +
      ' iDNA fees and ' +
      approximate(status.purchaseBudget) +
      ' iDNA purchases remain.'
    : ''
  $('readonly').textContent = preview
    ? 'Example only: transaction buttons are disabled.'
    : !status.canSign
    ? status.buyerOnly
      ? 'Read-only connection. Configure a local wallet key to enable buying.'
      : 'Read-only connection. Configure the local owner key to enable signing.'
    : ''
  $('wallet').textContent = 'Wallet: ' + status.wallet + ' iDNA'
  $('block').textContent = (preview ? 'Example' : 'Synced') + ' block ' + m.height
  $('worker').textContent = status.message
  $('withdraw').disabled = preview || !status.canSign || balance === 0n || reserved !== 0n
  renderOrders()
  navigate(tab)
}
function actionForm(operation, fields = {}) {
  const f = new FormData()
  f.set('operation', operation)
  for (const [k, v] of Object.entries(fields)) f.set(k, String(v))
  return f
}
async function prepare(operation, fields = {}, form) {
  selected = await api('/api/prepare', form || actionForm(operation, fields))
  showReview()
  message('Prepared locally. No transaction has been sent.')
}
function showReview() {
  const a = selected,
    list = $('review-values')
  list.replaceChildren()
  for (const [name, value] of [
    ['Action', operationLabels[a.operation] || a.operation],
    ['Account', a.signer],
    ['Contract', a.contract],
    ['Attached payment', idna(a.amount) + ' iDNA'],
    ['Maximum network fee', idna(a.maximumFee) + ' iDNA'],
    ['Maximum debit', idna((BigInt(a.amount) + BigInt(a.maximumFee)).toString()) + ' iDNA'],
  ]) {
    list.append(el('dt', name), el('dd', value))
  }
  $('review-copy').textContent =
    a.operation === 'depositStake'
      ? 'This deposit is reusable. It can be withdrawn after all orders, trades and disputes are closed.'
      : a.operation === 'acquire'
      ? 'Pool-funded seller proceeds remain held until verified delivery to a buyer.'
      : a.operation === 'challenge'
      ? 'If the seller proves correct delivery, your security deposit can be reduced by the dispute settlement.'
      : ''
  $('sign-vault-label').hidden = status.market.version !== 9 || a.operation !== 'queueSell' || status.automatic
  $('sign-vault-passphrase').value = ''
  $('sign').disabled = preview || a.attempted || !status.canSign
  $('review').showModal()
}
function renderOrders() {
  const m = status.market,
    box = $('orders')
  box.replaceChildren()
  const add = (title, copy, actions) => {
    const row = el('article', undefined, 'order')
    row.append(el('h3', title), el('p', copy))
    for (const a of actions) row.append(a)
    box.append(row)
  }
  for (const b of m.buys.filter((x) => x.buyer === m.identity && x.state === 1))
    add(
      'Buy order #' + b.id,
      'Waiting · maximum ' + approximate(b.maximum) + ' iDNA · expires at block ' + b.expires,
      [button('Cancel order', () => prepare('cancelBuy', { id: b.id }), preview)]
    )
  for (const s of m.sells.filter(
    (x) => x.seller === m.identity && (x.state === 1 || x.state === 2)
  ))
    add(
      'Sell order #' + s.id,
      s.state === 2
        ? 'Pool purchase reserved. Proceeds are held for delivery.'
        : 'Waiting · minimum ' + approximate(s.minimum) + ' iDNA',
      [
        button('Cancel order', () => prepare('cancelSell', { id: s.id }), preview),
        ...(s.state === 1 && m.poolCanAcquire
          ? [button('Review pool purchase', () => prepare('acquire'), preview)]
          : []),
      ]
    )
  const names = {
    1: 'Checking availability',
    2: 'Awaiting private delivery',
    3: 'Delivery ready for verification',
    4: 'Dispute · seller proof needed',
    5: 'Completed',
    6: 'Refunded / expired',
  }
  for (const t of m.trades.filter((x) => x.buyer === m.identity || x.seller === m.identity)) {
    const buyer = t.buyer === m.identity,
      actions = []
    if (buyer && t.state === 1 && m.height < t.readyBy)
      actions.push(button('Check availability', () => prepare('ready', { id: t.id }), preview))
    if (buyer && (t.state === 3 || t.state === 4) && m.height < t.proveBy) {
      actions.push(
        button(
          'Verify & accept',
          () => {
            navigate('market')
            $('unlock-area').scrollIntoView({ behavior: 'smooth' })
            message(
              'Enable automatic processing with your password for saved flip sets to verify and save every flip before acceptance.'
            )
          },
          preview
        ),
        button(
          'Dispute delivery',
          () => prepare('challenge', { id: t.id }),
          preview || t.state === 4 || m.height >= t.acceptBy
        )
      )
    }
    if (
      (t.state === 1 && m.height >= t.readyBy) ||
      (t.state === 2 && m.height >= t.deliverBy) ||
      ((t.state === 3 || t.state === 4) && m.height >= t.proveBy)
    )
      actions.push(
        button('Review expiry settlement', () => prepare('expireTrade', { id: t.id }), preview)
      )
    add(
      'Trade #' + t.id + ' · ' + (buyer ? 'Purchase' : 'Sale'),
      (names[t.state] || 'Unknown state') + ' · ' + approximate(t.price) + ' iDNA',
      actions
    )
    if (!buyer && (t.state === 4 || (t.state === 3 && m.height >= t.acceptBy))) {
      const row = el('form')
      const label = el('label', 'Completed delivery proof'),
        input = el('input')
      input.type = 'file'
      input.name = 'proof'
      input.required = true
      label.append(input)
      row.append(label)
      if (m.version === 9) {
        const contentLabel = el('label', 'Original-content proof')
        const contentInput = el('input')
        contentInput.type = 'file'; contentInput.name = 'contentProof'; contentInput.accept = '.json'; contentInput.required = true
        contentLabel.append(contentInput); row.append(contentLabel)
      }
      const b = el('button', 'Review seller proof')
      b.disabled = preview
      row.append(b)
      row.addEventListener('submit', (e) => {
        e.preventDefault()
        perform(async () => {
          const f = new FormData(row)
          f.set('operation', 'proveDelivery')
          f.set('id', t.id)
          await prepare('proveDelivery', {}, f)
        })
      })
      box.append(row)
    }
  }
  if (!box.children.length)
    box.append(el('p', 'No open orders or trades for this account.', 'empty'))
}
function clearPurchasedPreview() {
  purchasedPreview = null
  purchasedIndex = 0
  $('purchased-preview-images').replaceChildren()
  $('purchased-preview-meta').textContent = ''
  $('purchased-position').textContent = ''
}
function closePurchasedPreview() {
  if ($('purchased-preview').open) $('purchased-preview').close()
  clearPurchasedPreview()
}
function renderPurchasedFlip() {
  if (!purchasedPreview) return
  const { data, frames } = purchasedPreview,
    flip = data.flips[purchasedIndex],
    box = $('purchased-preview-images')
  box.replaceChildren()
  $('purchased-preview-title').textContent = 'Purchase #' + data.trade + ' · epoch ' + data.epoch
  $('purchased-preview-meta').textContent = 'Original CID: ' + flip.cid
  $('purchased-position').textContent = 'Flip ' + (purchasedIndex + 1) + ' of ' + data.count
  frames[purchasedIndex].forEach((sequence, sequenceIndex) => {
    const row = el('section', undefined, 'preview-sequence'),
      images = el('div', undefined, 'preview-images')
    row.append(el('h3', 'Sequence ' + (sequenceIndex + 1)))
    sequence.forEach((url, imageIndex) => {
      const img = el('img')
      img.src = url
      img.alt = 'Sequence ' + (sequenceIndex + 1) + ', frame ' + (imageIndex + 1)
      images.append(img)
    })
    row.append(images)
    box.append(row)
  })
  $('purchased-prev').disabled = purchasedIndex === 0
  $('purchased-next').disabled = purchasedIndex + 1 === data.count
}
async function openPurchasedPreview(id) {
  const identity = status.market.identity,
    data = preview ? demoPurchasedPreview(id) : await api('/api/v8/purchased-preview', { id })
  if (status.market.identity !== identity || !purchasedTrades(status.market).some((t) => t.id === id))
    throw Error('Purchase changed while loading. Refresh the market.')
  if (
    data.trade !== id ||
    !Number.isInteger(data.count) ||
    data.count < 1 ||
    !Array.isArray(data.flips) ||
    data.flips.length !== data.count
  )
    throw Error('Purchased flip preview is incomplete.')
  const frames = data.flips.map((flip) => purchasedFrames(flip))
  closePurchasedPreview()
  purchasedPreview = { data, frames }
  renderPurchasedFlip()
  $('purchased-preview').showModal()
  message('')
}
async function downloadPurchase(id) {
  const r = await fetch('/api/v8/download?trade=' + encodeURIComponent(id), {
    headers: { 'X-FOD-Token': token },
    cache: 'no-store',
  })
  if (!r.ok) throw Error((await r.text()).slice(0, 300))
  const url = URL.createObjectURL(await r.blob())
  const a = el('a')
  a.href = url
  a.download = 'flip-package-' + id + '.zip'
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 10000)
  message(
    'Package saved with encrypted keys and original encrypted flips. Keep your password for saved flip sets.'
  )
}
async function history() {
  if (preview) return
  const actions = await api('/api/actions')
  const box = $('history')
  box.replaceChildren()
  for (const a of actions.sort((a, b) => b.preparedAt.localeCompare(a.preparedAt))) {
    const row = el('div', undefined, 'order'),
      progress = actionProgress(a)
    row.append(
      el('p', (operationLabels[a.operation] || a.operation) + ' · ' + progress.state),
      el('p', progress.next, 'small')
    )
    if (a.submission?.transactionHash)
      row.append(el('p', 'Transaction: ' + a.submission.transactionHash, 'small tx-hash'))
    if (a.submission?.note)
      row.append(el('p', 'Chain note: ' + a.submission.note, 'small'))
    if (a.attempted && !a.submission?.included)
      row.append(
        button('Check chain result', async () => {
          await api('/api/reconcile', { id: a.id })
          await refresh()
          message('Chain result checked.')
        })
      )
    box.append(row)
  }
  if (!actions.length) box.append(el('p', 'No transactions prepared yet.', 'small'))
}
async function refresh() {
  let next
  try {
    next = preview ? demoStatus($('scenario').value, protocolVersion) : await api('/api/market')
  } catch (e) {
    if (ownNodeMode && token && e.message.includes('v7 node is not confirmed synced')) {
      $('workspace').hidden = true
      $('pairing').hidden = false
      $('pair-title').textContent = 'Waiting for your node to sync'
      $('pair-copy').textContent = 'Pairing complete. This page checks every 20 seconds and opens your market when the node is confirmed synced.'
      $('pair-form').hidden = true
      throw Error('Your local node is still syncing. Market actions will be available after sync completes.')
    }
    if (token && $('workspace').hidden) {
      $('pair-title').textContent = 'Local market unavailable'
      $('pair-copy').textContent = 'Check the local app, then enter its pairing code to reconnect.'
      $('pair-form').hidden = false
    }
    throw e
  }
  if (
    purchasedPreview &&
    (status?.market.identity !== next.market.identity ||
      !purchasedTrades(next.market).some((t) => t.id === purchasedPreview.data.trade))
  )
    closePurchasedPreview()
  status = next
  render()
  await history()
}
for (const b of document.querySelectorAll('[data-tab]'))
  b.addEventListener('click', () => navigate(b.dataset.tab))
$('pair-form').addEventListener('submit', (e) => {
  e.preventDefault()
  perform(async () => {
    const result = await api('/api/pair', { code: $('pair-code').value.trim() })
    token = result.token
    $('pair-code').value = ''
    await refresh()
    message('Connected.')
  })
})
$('refresh').addEventListener('click', () =>
  perform(async () => {
    await refresh()
    message(status.sellerClient || status.buyerOnly ? 'Updated from the relay.' : 'Updated from your local node.')
  })
)
$('collect').addEventListener('click', () =>
  perform(async () => {
    if (preview) {
      message('Live mode collects and checks encrypted flips from your own node.')
      return
    }
    await api('/api/v8/collect', {})
    await refresh()
    message('Package collection checked.')
  }, 'collect-error')
)
$('scenario').addEventListener('change', () =>
  perform(async () => {
    await refresh()
    message('')
  })
)
$('deposit-form').addEventListener('submit', (e) => {
  e.preventDefault()
  perform(async () => {
    const value = atoms($('deposit-amount').value)
    const target = BigInt(status.market.account.balance) + value
    await prepare('depositStake', {
      amount: value.toString(),
      target: (target > BigInt(status.market.account.target)
        ? target
        : BigInt(status.market.account.target)
      ).toString(),
    })
  })
})
$('unlock-form').addEventListener('submit', (e) => {
  e.preventDefault()
  perform(async () => {
    const f = new FormData(e.target)
    e.target.elements.passphrase.value = ''
    await api('/api/v8/unlock', {
      passphrase: f.get('passphrase'),
      perTrade: atoms(f.get('perTrade')).toString(),
      purchaseBudget: atoms(f.get('purchaseBudget')).toString(),
      feeBudget: atoms(f.get('feeBudget')).toString(),
    })
    pauseReason = ''
    await refresh()
    message('One-click trading enabled within your limits.')
  })
})
$('lock').addEventListener('click', () =>
  perform(async () => {
    await api('/api/v8/lock', {})
    pauseReason = 'You paused automatic trading.'
    await refresh()
    message('Automatic processing paused. Keep track of open trade deadlines.')
  })
)
async function order(side) {
  const terms = marketActions(status.market)
  const action = await api('/api/v8/order', {
    side,
    bundle: side === 'buy' ? '' : $('sell-package').value,
    maximum: terms.maximum,
    minimum: terms.minimum,
    capacity: terms.capacity,
  })
  await refresh()
  navigate('activity')
  message(
    action.submission?.success
      ? 'Order confirmed. Matching and delivery continue locally.'
      : 'Order saved; check its chain result.'
  )
}
$('buy').addEventListener('click', () => perform(() => order('buy')))
$('sell').addEventListener('click', () =>
  perform(() => order(marketActions(status.market).sellLabel === 'Sell to pool' ? 'pool' : 'sell'))
)
$('withdraw').addEventListener('click', () =>
  perform(() => prepare('withdrawStake', { amount: status.market.account.balance, exit: true }))
)
$('review-cancel').addEventListener('click', () => $('review').close())
$('purchased-preview-close').addEventListener('click', closePurchasedPreview)
$('purchased-preview').addEventListener('close', clearPurchasedPreview)
$('purchased-prev').addEventListener('click', () => {
  if (purchasedPreview && purchasedIndex > 0) {
    purchasedIndex--
    renderPurchasedFlip()
  }
})
$('purchased-next').addEventListener('click', () => {
  if (purchasedPreview && purchasedIndex + 1 < purchasedPreview.data.count) {
    purchasedIndex++
    renderPurchasedFlip()
  }
})
$('sign').addEventListener('click', () =>
  perform(async () => {
    if (!selected) return
    const a = selected
    $('sign').disabled = true
    $('review').close()
    await api('/api/submit', { id: a.id, reviewHash: a.reviewHash, passphrase: $('sign-vault-passphrase').value })
    $('sign-vault-passphrase').value = ''
    const result = await api('/api/reconcile', { id: a.id })
    await refresh()
    navigate('activity')
    message(result.submission?.included
      ? result.submission.success
        ? 'Transaction confirmed in a block. See Activity.'
        : 'Transaction failed on chain. See Activity.'
      : 'Transaction sent. Waiting for a block receipt; see Activity.')
  })
)
$('protect-cancel').addEventListener('click', () => $('protect').close())
$('protect-form').addEventListener('submit', (e) => {
  e.preventDefault()
  perform(async () => {
    const f = new FormData(e.target)
    await api('/api/v8/protect', { bundle: packageToProtect, passphrase: f.get('passphrase') })
    e.target.reset()
    $('protect').close()
    await refresh()
    message('Package verified and keys protected locally.')
  })
})
$('import-form').addEventListener('submit', (e) => {
  e.preventDefault()
  perform(async () => {
    await api('/api/v8/import', new FormData(e.target))
    e.target.reset()
    await refresh()
    message('Canonical validation evidence added.')
  }, 'import-error')
})
$('registration-form').addEventListener('submit', (e) => {
  e.preventDefault()
  perform(async () => {
    const f = new FormData(e.target)
    f.set('operation', 'registerBundle')
    await prepare('registerBundle', {}, f)
  })
})
try {
  const mode = await fetch('/api/mode', { cache: 'no-store' }).then((r) => r.json())
  preview = mode.preview === true
  ownNodeMode = !preview && mode.buyerOnly !== true && mode.sellerClient !== true
  protocolVersion = mode.version === 9 ? 9 : 8
  $('connection').textContent = (mode.buyerOnly === true ? 'Buyer wallet' : mode.sellerClient === true
    ? 'Seller via shared node' : 'Local market') + ' · v' + protocolVersion
  if (mode.buyerOnly === true) {
    document.title = 'Flips on Demand · Buyer wallet'
    $('pair-kicker').textContent = 'BUY FLIP KEYS'
    $('pair-title').textContent = 'Connect your buyer wallet'
    $('pair-copy').textContent = 'Enter the pairing code shown by the app on this computer. Your wallet key stays here.'
  } else if (mode.sellerClient === true) {
    document.title = 'Flips on Demand · Seller'
    $('pair-kicker').textContent = 'SELL YOUR FLIPS'
    $('pair-title').textContent = 'Connect your seller app'
    $('pair-copy').textContent = 'Enter the pairing code shown by the app on this computer. Your identity key stays here.'
  }
  if (preview) {
    $('preview').hidden = false
    await refresh()
  }
  setInterval(() => {
    if (!preview && token && !busy && !$('review').open && !$('protect').open && !$('purchased-preview').open)
      perform(async () => {
        await refresh()
        message('')
      })
  }, 20000)
} catch (e) {
  message('Local app unavailable: ' + e.message)
}
