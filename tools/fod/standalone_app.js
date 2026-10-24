(() => {
  'use strict';
  let token = null;
  const $ = (id) => document.getElementById(id);
  let selected = null;
  let market = null;
  let mode = 'buy';
  let previewMode = false;
  let demoScenario = 'private';
  let unresolvedAttempt = null;
  const demoActions = [];
  const actionFields = {
    buy: ['offerId', 'session'], buyPublicSeed: ['offerId'], confirm: ['orderId', 'receiptId'],
    challenge: ['orderId'], refund: ['orderId'], qualify: ['offerId', 'session', 'proof', 'registration-host'],
    proveBundle: ['offerId', 'session', 'proof', 'registration-host'], list: ['offerId', 'minimumPayout', 'session', 'vault', 'passphrase'],
    relist: ['offerId', 'minimumPayout'], startFallback: ['orderId', 'session', 'proof', 'registration-host'],
    proveAndReveal: ['orderId', 'vault', 'passphrase'], seedBundle: ['offerId', 'vault', 'passphrase'],
    topUpBond: ['offerId', 'amount'], cancel: ['offerId'], expireHead: ['offerId'], slashUnproved: ['orderId']
  };
  const modeActions = {
    buy: ['buy', 'buyPublicSeed'],
    sell: ['qualify', 'proveBundle', 'list', 'relist', 'seedBundle', 'topUpBond', 'cancel'],
    orders: ['confirm', 'challenge', 'refund', 'startFallback', 'proveAndReveal', 'expireHead', 'slashUnproved']
  };
  const short = (value) => value ? `${value.slice(0, 10)}…${value.slice(-8)}` : '—';
  const idna = (base) => {
    if (!/^[0-9]+$/.test(String(base))) return '—';
    const value = BigInt(base);
    const whole = value / 1000000000000000000n;
    const fraction = String(value % 1000000000000000000n).padStart(18, '0').replace(/0+$/, '');
    return `${whole}${fraction ? '.' + fraction : ''} iDNA`;
  };
  const setMessage = (value) => { $('message').textContent = value || ''; };
  const demoMarket = () => {
    const seller = demoScenario === 'seller' || demoScenario === 'fallback';
    const resale = demoScenario === 'resale';
    const seed = demoScenario === 'seed';
    return {
      blockHeight: 8400000, activeIdentity: (seller ? '5' : '1').repeat(40),
      contract: '2'.repeat(40), codeHash: '3'.repeat(64),
      primaryUnits: resale || seed ? '0' : '1', secondaryUnits: resale || seed ? '1' : '0',
      quote: '1000000000000000000000',
      nextOffer: {offerId: '4'.repeat(64), author: '5'.repeat(40), epoch: 232, covered: 4,
        publicSeed: seed, everSold: resale || seed, originalCids: [],
        status: resale || seed ? 'queued-secondary' : 'queued-primary'},
      openOrders: resale || seed ? [] : [{orderId: '42', offerId: '6'.repeat(64),
        author: '5'.repeat(40), buyer: '1'.repeat(40),
        state: demoScenario === 'fallback' ? 'fallback pending' : 'funded',
        fundedAt: demoScenario === 'expired' ? 8380000 : 8390000,
        proofDeadline: demoScenario === 'expired' ? 8392960 : 8402960,
        challenged: demoScenario === 'fallback'}]
    };
  };
  const demoImage = (index, panel) => {
    const colors = ['#31485f', '#4c5269', '#445e59', '#695447'];
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="180" height="120" viewBox="0 0 180 120"><rect width="180" height="120" fill="${colors[index]}"/><circle cx="${panel ? 122 : 58}" cy="49" r="24" fill="#e5b85c"/><path d="M0 102 Q90 55 180 102 V120 H0" fill="#1d2b38"/><text x="90" y="111" text-anchor="middle" font-size="10" fill="white" font-family="sans-serif">SYNTHETIC EXAMPLE</text></svg>`;
    return `data:image/svg+xml,${encodeURIComponent(svg)}`;
  };
  const demoPreview = () => ({offerId: '4'.repeat(64), count: 4,
    flips: Array.from({length: 4}, (_, index) => ({cid: `synthetic-flip-${index + 1}`,
      images: [demoImage(index, 0), demoImage(index, 1)]}))});
  const demoPrepare = (body) => {
    const operation = body.get('operation');
    const price = operation === 'buy' || operation === 'buyPublicSeed' ? market.quote : '';
    const amount = operation === 'buy' ? '1025000000000000000000' : operation === 'buyPublicSeed' ? price : '';
    const action = {id: String(demoActions.length + 1).padStart(32, '0'), reviewHash: 'f'.repeat(64),
      operation, contract: market.contract, codeHash: market.codeHash, signer: market.activeIdentity,
      offerId: body.get('offerId') || market.nextOffer.offerId, orderId: body.get('orderId') || '',
      epoch: 232, coveredFlips: 4, amount, price,
      maximumFee: amount ? '10000000000000000' : '',
      preparedAt: new Date().toISOString(), attempted: false};
    demoActions.push(action);
    return action;
  };
  const request = async (path, options = {}) => {
    if (previewMode) {
      if (path === '/api/market') return demoMarket();
      if (path === '/api/preview') return demoPreview();
      if (path === '/api/actions') return demoActions.slice();
      if (path === '/api/prepare') return demoPrepare(options.body);
      throw new Error('Synthetic preview cannot read private files or send transactions.');
    }
    if (!token) throw new Error('Pair this browser first.');
    const response = await fetch(path, {cache: 'no-store', ...options,
      headers: {...options.headers, 'X-FOD-Token': token}});
    if (!response.ok) throw new Error((await response.text()).trim());
    return response.json();
  };
  const cell = (dl, name, value) => {
    const dt = document.createElement('dt'); dt.textContent = name;
    const dd = document.createElement('dd'); dd.textContent = String(value ?? '—');
    dl.append(dt, dd);
  };
  const setMode = (next) => {
    const previous = mode;
    mode = next;
    for (const button of document.querySelectorAll('.mode-nav button')) {
      const active = button.dataset.mode === next;
      button.classList.toggle('active', active);
      if (active) button.setAttribute('aria-current', 'page');
      else button.removeAttribute('aria-current');
    }
    $('market-section').hidden = next === 'orders';
    $('orders-section').hidden = next !== 'orders';
    $('action-title').textContent = next === 'buy' ? 'Buy the next bundle' : next === 'sell' ? 'Sell a bundle' : 'Order actions';
    $('action-hint').textContent = next === 'buy' ? 'The oldest waiting first sale has priority. Resales follow in their own queue. The public seed is repeatable with keys already public.' :
      next === 'sell' ? 'Prepare a listing with your original bundle and private evidence. The app estimates the exact collateral before signing.' :
      'Verify a received private file before confirming. Challenge, fallback, and recovery actions are available when the contract permits them.';
    const operation = $('operation');
    for (const option of operation.options) {
      option.disabled = !modeActions[next].includes(option.value);
      option.hidden = option.disabled;
    }
    if (!modeActions[next].includes(operation.value)) operation.value = next === 'buy' && market?.nextOffer?.publicSeed ? 'buyPublicSeed' : modeActions[next][0];
    if (next === 'buy' && market?.nextOffer) $('offer-id').value = market.nextOffer.offerId;
    if (next === 'sell' && previous === 'buy') $('offer-id').value = '';
    updateFields();
  };
  const disclosure = (action) => ['seedBundle', 'proveAndReveal'].includes(action.operation) ?
    'Public key disclosure: this action permanently publishes the bundle unlocking keys on-chain.' :
    'This action does not publish the bundle unlocking keys.';
  const costRows = (dl, action) => {
    if (!/^[0-9]+$/.test(action.amount) || !/^[0-9]+$/.test(action.maximumFee)) return;
    const amount = BigInt(action.amount), fee = BigInt(action.maximumFee);
    if (action.price && /^[0-9]+$/.test(action.price)) {
      cell(dl, 'Bundle price', idna(action.price));
      if (action.operation === 'buy' && amount >= BigInt(action.price)) {
        cell(dl, 'Refundable buyer bond', idna((amount - BigInt(action.price)).toString()));
      }
    }
    cell(dl, 'Attached contract value', `${idna(action.amount)} (${action.amount} base units)`);
    if (['list', 'relist'].includes(action.operation)) cell(dl, 'Seller collateral', 'Included in attached value; unproved content also requires 1,000 iDNA fraud collateral.');
    cell(dl, 'Maximum network fee', `${idna(action.maximumFee)} (${action.maximumFee} base units)`);
    cell(dl, 'Maximum needed in wallet', idna((amount + fee).toString()));
  };
  const showReview = (action) => {
    selected = action;
    $('review-section').hidden = false;
    const review = $('review'); review.replaceChildren(); review.classList.remove('empty');
    const dl = document.createElement('dl');
    cell(dl, 'Action', action.operation);
    cell(dl, 'Saved action ID', action.id);
    cell(dl, 'Bundle', action.offerId || '—');
    if (action.epoch) cell(dl, 'Validation epoch', action.epoch);
    if (action.coveredFlips) cell(dl, 'Flips in bundle', action.coveredFlips);
    if (action.orderId) cell(dl, 'Order', action.orderId);
    if (action.author) cell(dl, 'Author', action.author);
    if (action.buyer) cell(dl, 'Buyer', action.buyer);
    cell(dl, 'Contract', action.contract);
    cell(dl, 'Code hash', action.codeHash);
    cell(dl, 'Signing identity', action.signer);
    costRows(dl, action);
    if (previewMode) cell(dl, 'Estimate', 'Illustrative only. No node or live fee quote was consulted.');
    if (action.minimumPayout && action.minimumPayout !== '0') cell(dl, 'Minimum seller payout', idna(action.minimumPayout));
    cell(dl, 'Prepared at', action.preparedAt);
    cell(dl, 'Review digest', action.reviewHash);
    cell(dl, 'Key disclosure', disclosure(action));
    if (action.attempted) cell(dl, 'Attempt', action.submission?.transactionHash ? `Hash ${action.submission.transactionHash}` :
      `Nonce ${action.attemptNonce || 'unknown'} was reserved; no saved hash is available. Reconcile manually before another signing attempt.`);
    review.append(dl);
    if (action.operation === 'buy') $('receive-form').querySelector('[name="purchaseId"]').value = action.id;
    $('uncertain-warning').hidden = !(unresolvedAttempt || (action.attempted && !action.submission?.included));
    $('submit').disabled = previewMode || action.attempted || !!unresolvedAttempt || !action.reviewHash;
    $('reconcile').disabled = previewMode || !action.attempted || !!action.submission?.included;
    if (previewMode) $('submit').textContent = 'Synthetic preview · signing unavailable';
  };
  const refreshPreview = async (offer) => {
    const box = $('market-preview'); box.replaceChildren();
    if (!offer) return;
    box.textContent = 'Loading flip previews…';
    try {
      const preview = await request('/api/preview');
      if (market?.nextOffer?.offerId !== preview.offerId || offer.offerId !== preview.offerId) return;
      box.replaceChildren();
      const title = document.createElement('div'); title.className = 'preview-caption';
      title.textContent = previewMode ? `Synthetic preview · ${preview.count} example flips` :
        `Unverified public preview · ${preview.count} original flips`;
      box.append(title);
      const grid = document.createElement('div'); grid.className = 'preview-grid';
      let pictured = 0;
      for (const [index, flip] of preview.flips.entries()) {
        const item = document.createElement('div'); item.className = 'preview-item';
        const pictures = document.createElement('div'); pictures.className = 'preview-pictures';
        if (Array.isArray(flip.images) && flip.images.length === 2) {
          for (const [panel, source] of flip.images.entries()) {
            const img = document.createElement('img'); img.src = source;
            img.alt = `Flip ${index + 1}, picture ${panel + 1}`; img.loading = 'lazy';
            pictures.append(img);
          }
          pictured++;
        } else { pictures.textContent = 'Image unavailable'; }
        const cid = document.createElement('div'); cid.className = 'preview-cid';
        cid.textContent = `Flip ${index + 1} · ${short(flip.cid)}`; cid.title = flip.cid;
        item.append(pictures, cid); grid.append(item);
      }
      box.append(grid);
      const note = document.createElement('p'); note.className = 'subtle';
      note.textContent = previewMode ? 'Example only. No original flip or chain state was loaded.' : pictured ?
        'Public explorer pictures help browsing. They do not prove complete content or private delivery.' :
        'Explorer pictures are unavailable. Original CIDs were checked against the contract inventory.';
      box.append(note);
    } catch { if (market?.nextOffer?.offerId === offer.offerId) box.textContent = 'Preview unavailable; the market remains usable.'; }
  };
  const refreshMarket = async () => {
    const state = $('market-state'); state.textContent = 'Reading your node…';
    try {
      market = await request('/api/market');
      state.replaceChildren();
      const price = document.createElement('div'); price.className = 'quote'; price.textContent = market.quote ? idna(market.quote) : 'No bundle available';
      const quoteLabel = document.createElement('div'); quoteLabel.className = 'subtle';
      quoteLabel.textContent = market.quote ? previewMode ?
        'Illustrative price · no live quote · capped at 1,000 iDNA' :
        `Price at block ${market.blockHeight} · changes with queued supply and reserve · capped at 1,000 iDNA` : '';
      const title = document.createElement('div'); title.className = 'market-kind';
      title.textContent = market.nextOffer ? `${market.nextOffer.publicSeed ? 'Public seed · repeat purchase · keys already public' :
        market.nextOffer.everSold ? 'Resale · author receives half; rest enters reserve' :
          'Private first sale · oldest waiting first · author receives the price'} · ${market.nextOffer.covered} flips` : 'The queue is empty';
      const meta = $('market-details'); meta.replaceChildren();
      const block = document.createElement('span');
      block.textContent = `${previewMode ? 'Synthetic' : 'Synced node'} block ${market.blockHeight}`;
      meta.append(block);
      for (const [label, value] of [['bundle', market.nextOffer?.offerId], ['identity', market.activeIdentity],
        ['contract', market.contract], ['code hash', market.codeHash]]) {
        if (!value) continue;
        const span = document.createElement('span'); span.className = 'copyable';
        const code = document.createElement('code'); code.textContent = `${label} ${short(value)}`; code.title = value;
        const button = document.createElement('button'); button.type = 'button';
        button.textContent = 'Copy'; button.setAttribute('aria-label', `Copy ${label}`);
        button.addEventListener('click', async () => {
          try { await navigator.clipboard.writeText(value); setMessage(`${label} copied.`); }
          catch { setMessage(`Copy unavailable. Full ${label} is shown in the signing review.`); }
        });
        span.append(code, button); meta.append(span);
      }
      state.append(price, quoteLabel, title);
      void refreshPreview(market.nextOffer);
      const ordersBox = $('open-orders'); ordersBox.replaceChildren();
      $('order-status').textContent = 'Select an order to see its next step and on-chain deadline.';
      $('order-status').classList.remove('risk');
      $('dm-form').hidden = true;
      $('receive-form').hidden = true;
      const mine = (market.openOrders || []).filter((order) =>
        order.buyer === market.activeIdentity || order.author === market.activeIdentity);
      if (mine.length) {
        for (const order of mine.slice(0, 10)) {
          const button = document.createElement('button'); button.type = 'button';
          const seller = order.author === market.activeIdentity;
          const invalidDeliveryKey = order.invalidDeliveryKey === true;
          button.textContent = `Order ${order.orderId} · ${order.state} · ${seller ? 'sell' : 'buy'} · ${short(order.offerId)}${invalidDeliveryKey ? ' · invalid delivery key' : ''}`;
          button.addEventListener('click', () => {
            setMode('orders');
            $('order-status').classList.toggle('risk', invalidDeliveryKey);
            const deadline = Number(order.proofDeadline);
            const expired = Number.isSafeInteger(deadline) && market.blockHeight >= deadline;
            const remaining = Number.isSafeInteger(deadline) ? Math.max(0, deadline - market.blockHeight) : null;
            const timing = `Funded at block ${order.fundedAt}. ${remaining} blocks remain until the proof deadline at block ${deadline}. Refresh for current chain state; block timing varies.`;
            const warning = invalidDeliveryKey ? 'Invalid buyer delivery key on chain. Private delivery cannot be created or verified; do not confirm this order. ' : '';
            if (remaining === null) $('order-status').textContent = `${warning}Refresh the order before acting; deadline data is unavailable.`;
            else if (expired) $('order-status').textContent = `${warning}Proof deadline reached at block ${deadline}. Anyone may claim a refund; the locked price, buyer bond, and seller delivery bond go to the buyer. Funded refund remains untested in the pilot.`;
            else if (order.state === 'fallback pending') $('order-status').textContent = `${warning}${timing} Seller fallback is pending. A valid proof publishes the bundle keys for everyone; if it is not completed by the deadline, anyone may claim a refund.`;
            else $('order-status').textContent = `${warning}${timing} ${order.challenged ? 'Buyer challenge recorded. ' : ''}${invalidDeliveryKey ?
              (seller ? 'Use the allowed on-chain fallback path.' : order.challenged ? 'Wait for fallback or the refund deadline.' : 'You may challenge the failed delivery and monitor fallback or refund.') :
              (seller ? 'Deliver privately or use the allowed fallback path.' : 'Verify the encrypted file before confirming, or challenge if delivery fails.')}`;
            const form = $(seller ? 'dm-form' : 'receive-form');
            $('dm-form').hidden = !seller || expired || order.state === 'fallback pending' || invalidDeliveryKey;
            $('receive-form').hidden = seller || expired || order.state === 'fallback pending' || invalidDeliveryKey;
            form.querySelector('[name="orderId"]').value = order.orderId;
            $('prepare-form').querySelector('[name="orderId"]').value = order.orderId;
            $('operation').value = expired ? 'refund' : seller ?
              (order.state === 'fallback pending' ? 'proveAndReveal' : 'startFallback') :
              invalidDeliveryKey ? (order.challenged ? 'refund' : 'challenge') : 'confirm';
            updateFields();
            (form.hidden ? $('trade-section') : form).scrollIntoView({behavior: 'smooth', block: 'center'});
          });
          ordersBox.append(button);
        }
      } else ordersBox.textContent = 'No open orders for the active identity.';
      if (market.nextOffer && ['buy', 'buyPublicSeed'].includes($('operation').value)) {
        $('operation').value = market.nextOffer.publicSeed ? 'buyPublicSeed' : 'buy';
        $('offer-id').value = market.nextOffer.offerId;
        updateFields();
      }
      if (mode === 'buy' && market.nextOffer) setMode('buy');
    } catch (error) { market = null; state.textContent = `Node state unavailable: ${error.message}`; $('market-preview').replaceChildren(); $('open-orders').replaceChildren(); }
  };
  const loadActions = async () => {
    try {
      const list = await request('/api/actions');
      unresolvedAttempt = list.find((action) => action.signer === market?.activeIdentity &&
        action.attempted && !action.submission?.included) || null;
      const box = $('actions'); box.replaceChildren();
      if (!list.length) { box.textContent = 'No saved actions.'; return; }
      for (const action of list.reverse()) {
        const button = document.createElement('button'); button.type = 'button';
        button.textContent = `${action.operation}${action.orderId ? ` · order ${action.orderId}` : ''} · ${action.preparedAt} · ${action.submission?.included ? (action.submission.success ? 'included' : 'failed') : action.attempted ? 'needs reconciliation' : 'unsigned'}`;
        button.addEventListener('click', () => showReview(action)); box.append(button);
      }
      if (selected) showReview(list.find((action) => action.id === selected.id) || selected);
      if (unresolvedAttempt && !selected?.attempted) setMessage('An earlier transaction needs reconciliation. Open it under Saved attempts before signing again.');
    } catch (error) { setMessage(error.message); }
  };
  $('pair-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const button = form.querySelector('button'); button.disabled = true;
    $('pair-message').textContent = '';
    try {
      const response = await fetch('/api/pair', {method:'POST', cache:'no-store',
        headers:{'Content-Type':'application/json'}, body:JSON.stringify({code:$('pair-code').value.trim()})});
      if (!response.ok) throw new Error((await response.text()).trim());
      const result = await response.json();
      if (!/^[0-9a-f]{32}$/.test(result.token)) throw new Error('Pairing response was invalid.');
      token = result.token;
      $('pair-code').value = '';
      $('pairing').hidden = true; $('workspace').hidden = false;
      await refreshMarket(); await loadActions();
    } catch (error) { $('pair-message').textContent = error.message; }
    finally { button.disabled = false; }
  });
  $('prepare-form').addEventListener('submit', async (event) => {
    event.preventDefault(); setMessage('Preparing and estimating against your node…');
    const form = event.currentTarget;
    $('prepare').disabled = true;
    try {
      const body = new FormData(form);
      const action = await request('/api/prepare', {method:'POST', body});
      form.querySelector('[name="passphrase"]').value = '';
      showReview(action); $('review-section').scrollIntoView({behavior:'smooth', block:'start'});
      setMessage(previewMode ? 'Synthetic action prepared. Signing is unavailable in preview mode.' :
        'Unsigned action prepared. Review the destination, attached value, fee ceiling, identity, and disclosure before signing.');
      await loadActions();
    } catch (error) { setMessage(error.message); }
    finally { form.querySelector('[name="passphrase"]').value = ''; $('prepare').disabled = false; }
  });
  $('submit').addEventListener('click', () => {
    if (!selected || selected.attempted || unresolvedAttempt || previewMode) return;
    const dialog = $('sign-dialog');
    $('sign-summary').replaceChildren($('review').querySelector('dl').cloneNode(true));
    const disclosureWarning = ['seedBundle', 'proveAndReveal'].includes(selected.operation);
    $('sign-disclosure').hidden = !disclosureWarning;
    $('sign-disclosure').textContent = disclosureWarning ? disclosure(selected) : '';
    $('sign-ack').checked = false;
    $('confirm-sign').disabled = true;
    dialog.showModal();
    $('sign-ack').focus();
  });
  $('sign-ack').addEventListener('change', () => { $('confirm-sign').disabled = !$('sign-ack').checked; });
  $('cancel-sign').addEventListener('click', () => $('sign-dialog').close());
  $('confirm-sign').addEventListener('click', async () => {
    if (!$('sign-ack').checked || !selected || selected.attempted || unresolvedAttempt || previewMode) return;
    $('sign-dialog').close();
    $('submit').disabled = true; setMessage('Signing and sending. Do not retry if the result is uncertain.');
    try {
      const action = await request('/api/submit', {method:'POST', headers:{'Content-Type':'application/json'},
        body:JSON.stringify({id:selected.id, reviewHash:selected.reviewHash})});
      showReview(action); setMessage(action.submission?.included ? 'Transaction included. Check the contract result before continuing.' : 'Sent; inclusion is not yet confirmed. Reconcile the saved hash before any retry.');
    } catch (error) {
      if (selected) showReview({...selected, attempted:true});
      setMessage(`Signing outcome needs reconciliation: ${error.message}`);
    }
    finally { await loadActions(); }
  });
  $('reconcile').addEventListener('click', async () => {
    if (!selected?.attempted) return;
    setMessage('Checking the saved transaction hash with your node…');
    try {
      const action = await request('/api/reconcile', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({id:selected.id})});
      showReview(action);
      setMessage(action.submission?.included ? (action.submission.success ? 'Included; contract call succeeded.' : 'Included; contract call failed. Do not retry this action.') : 'Not included yet. Check again later; do not retry while the outcome is uncertain.');
      await loadActions();
    } catch (error) { setMessage(`Reconciliation needs manual review: ${error.message}`); }
  });
  $('dm-form').addEventListener('submit', async (event) => {
    event.preventDefault(); setMessage('Creating encrypted delivery locally…');
    const form = event.currentTarget;
    const button = form.querySelector('button'); button.disabled = true;
    try {
      const result = await request('/api/private/dm', {method:'POST', body:new FormData(form)});
      form.querySelector('[name="passphrase"]').value = '';
      const response = await fetch(`/api/private/download/${result.id}`, {headers:{'X-FOD-Token':token}, cache:'no-store'});
      if (!response.ok) throw new Error((await response.text()).trim());
      const blob = await response.blob(); const url = URL.createObjectURL(blob);
      const link = document.createElement('a'); link.href = url; link.download = `flips-ciphertext-${result.id}.txt`;
      link.click(); window.setTimeout(() => URL.revokeObjectURL(url), 30_000);
      $('dm-result').textContent = 'Encrypted file downloaded. Send it privately to the buyer through your chosen channel.';
      setMessage('No ciphertext was sent by the app.');
    } catch (error) { setMessage(error.message); }
    finally { form.querySelector('[name="passphrase"]').value = ''; button.disabled = false; }
  });
  $('receive-form').addEventListener('submit', async (event) => {
    event.preventDefault(); setMessage('Verifying the encrypted delivery and original flips…');
    const form = event.currentTarget;
    const button = form.querySelector('button'); button.disabled = true;
    try {
      const result = await request('/api/private/receive', {method:'POST', body:new FormData(form)});
      form.querySelector('[name="passphrase"]').value = '';
      $('receive-result').textContent = `Verified receipt saved locally. Receipt ID: ${result.id}`;
      $('prepare-form').querySelector('[name="receiptId"]').value = result.id;
      $('prepare-form').querySelector('[name="orderId"]').value = form.querySelector('[name="orderId"]').value;
      $('operation').value = 'confirm';
      setMode('orders');
      updateFields();
      setMessage('Delivery verified. Review the confirm action separately before signing.');
    } catch (error) { setMessage(error.message); }
    finally { form.querySelector('[name="passphrase"]').value = ''; button.disabled = false; }
  });
  $('refresh').addEventListener('click', refreshMarket);
  $('demo-scenario').addEventListener('change', async (event) => {
    demoScenario = event.target.value;
    demoActions.length = 0;
    selected = null;
    $('review-section').hidden = true;
    await refreshMarket(); await loadActions();
    setMode(demoScenario === 'seller' || demoScenario === 'fallback' ? 'orders' : 'buy');
  });
  $('reload-actions').addEventListener('click', loadActions);
  $('operation').addEventListener('change', () => {
    if (market?.nextOffer && ['buy', 'buyPublicSeed'].includes($('operation').value)) {
      $('operation').value = market.nextOffer.publicSeed ? 'buyPublicSeed' : 'buy';
      $('offer-id').value = market.nextOffer.offerId;
    }
    updateFields();
  });
  $('prepare-form').querySelector('[name="receiptId"]').addEventListener('input', updateFields);
  for (const button of document.querySelectorAll('.mode-nav button')) button.addEventListener('click', () => setMode(button.dataset.mode));
  function updateFields() {
    const visible = new Set(actionFields[$('operation').value] || []);
    for (const label of $('prepare-form').querySelectorAll('[data-for]')) {
      const show = visible.has(label.dataset.for);
      label.hidden = !show;
      const input = label.querySelector('input');
      input.disabled = !show || (previewMode && (input.type === 'file' || input.type === 'password'));
    }
    $('evidence').hidden = previewMode || !['session', 'proof', 'registration-host', 'vault'].some((field) => visible.has(field));
    const confirmNeedsReceipt = $('operation').value === 'confirm' && !$('prepare-form').querySelector('[name="receiptId"]').value.trim();
    $('prepare').disabled = confirmNeedsReceipt;
    $('prepare').title = confirmNeedsReceipt ? 'Verify the seller ciphertext locally to create a saved receipt first.' : '';
  }
  const initialize = async () => {
    try {
      const response = await fetch('/api/mode', {cache:'no-store'});
      if (response.ok) previewMode = (await response.json()).preview === true;
    } catch { /* Live pairing remains available if the mode check is unavailable. */ }
    if (previewMode) {
      token = 'synthetic-preview';
      $('pairing').hidden = true; $('workspace').hidden = false;
      $('live-risk').hidden = true; $('preview-banner').hidden = false;
      $('demo-scenario-wrap').hidden = false;
      $('connection-label').textContent = 'Read-only example · No node';
      for (const form of [$('dm-form'), $('receive-form')]) for (const control of form.elements) control.disabled = true;
      $('refresh').textContent = 'Reset example';
      setMessage('Explore buyer and seller views. All figures and orders are synthetic; signing and private files are unavailable.');
      await refreshMarket(); await loadActions();
    }
    setMode('buy');
  };
  void initialize();
})();
