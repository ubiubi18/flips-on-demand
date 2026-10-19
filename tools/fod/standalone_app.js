(() => {
  'use strict';
  let token = null;
  const $ = (id) => document.getElementById(id);
  let selected = null;
  let market = null;
  const actionFields = {
    buy: ['offerId', 'session'], buyPublicSeed: ['offerId'], confirm: ['orderId', 'receiptId', 'delivery'],
    challenge: ['orderId'], refund: ['orderId'], qualify: ['offerId', 'session', 'proof', 'registration-host'],
    proveBundle: ['offerId', 'session', 'proof', 'registration-host'], list: ['offerId', 'minimumPayout', 'session', 'vault', 'passphrase'],
    relist: ['offerId', 'minimumPayout'], startFallback: ['orderId', 'session', 'proof', 'registration-host'],
    proveAndReveal: ['orderId', 'vault', 'passphrase'], seedBundle: ['offerId', 'vault', 'passphrase'],
    topUpBond: ['offerId', 'amount'], cancel: ['offerId'], expireHead: ['offerId'], slashUnproved: ['orderId']
  };
  const short = (value) => value ? `${value.slice(0, 10)}…${value.slice(-8)}` : '—';
  const idna = (base) => {
    if (!/^[0-9]+$/.test(String(base))) return '—';
    const value = BigInt(base);
    const whole = value / 1000000000000000000n;
    const fraction = String(value % 1000000000000000000n).padStart(18, '0').slice(0, 4).replace(/0+$/, '');
    return `${whole}${fraction ? '.' + fraction : ''} iDNA`;
  };
  const setMessage = (value) => { $('message').textContent = value || ''; };
  const request = async (path, options = {}) => {
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
  const showReview = (action) => {
    selected = action;
    const review = $('review'); review.replaceChildren(); review.classList.remove('empty');
    const dl = document.createElement('dl');
    cell(dl, 'Action', action.operation);
    cell(dl, 'Saved action ID', action.id);
    cell(dl, 'Bundle', action.offerId ? short(action.offerId) : '—');
    if (action.epoch) cell(dl, 'Validation epoch', action.epoch);
    if (action.coveredFlips) cell(dl, 'Flips in bundle', action.coveredFlips);
    if (action.orderId) cell(dl, 'Order', action.orderId);
    if (action.author) cell(dl, 'Author', action.author);
    if (action.buyer) cell(dl, 'Buyer', action.buyer);
    cell(dl, 'Contract', action.contract);
    cell(dl, 'Code hash', action.codeHash);
    cell(dl, 'Signing identity', action.signer);
    cell(dl, 'Attached value', `${idna(action.amount)} (${action.amount} base units)`);
    if (action.price) cell(dl, 'Price', idna(action.price));
    if (action.minimumPayout && action.minimumPayout !== '0') cell(dl, 'Minimum seller payout', idna(action.minimumPayout));
    cell(dl, 'Maximum network fee', `${idna(action.maximumFee)} (${action.maximumFee} base units)`);
    cell(dl, 'Prepared at', action.preparedAt);
    cell(dl, 'Review digest', action.reviewHash);
    if (['seedBundle', 'proveAndReveal'].includes(action.operation)) cell(dl, 'Disclosure', 'This action publishes bundle keys.');
    if (action.attempted) cell(dl, 'Attempt', action.submission?.transactionHash ? `Hash ${action.submission.transactionHash}` : 'Attempt recorded; reconcile before retry.');
    review.append(dl);
    if (action.operation === 'buy') $('receive-form').querySelector('[name="purchaseId"]').value = action.id;
    $('submit').disabled = action.attempted || !action.reviewHash;
    $('reconcile').disabled = !action.attempted || !action.submission?.transactionHash;
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
      title.textContent = `Bundle preview · ${preview.count} original flips`;
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
      note.textContent = pictured ? 'Pictures come from the public Idena explorer. They are for browsing and do not prove private delivery.' :
        'Explorer pictures are unavailable. The original CIDs above were verified against the contract inventory.';
      box.append(note);
    } catch { if (market?.nextOffer?.offerId === offer.offerId) box.textContent = 'Preview unavailable; the market remains usable.'; }
  };
  const refreshMarket = async () => {
    const state = $('market-state'); state.textContent = 'Reading your node…';
    try {
      market = await request('/api/market');
      state.replaceChildren();
      const price = document.createElement('div'); price.className = 'quote'; price.textContent = market.quote ? idna(market.quote) : 'No bundle available';
      const quoteLabel = document.createElement('div'); quoteLabel.className = 'subtle'; quoteLabel.textContent = market.quote ? `Quote at block ${market.blockHeight} · changes with the queue and reserve` : '';
      const title = document.createElement('div'); title.textContent = market.nextOffer ? `${market.nextOffer.everSold ? 'Resale' : 'First sale'} · ${market.nextOffer.covered} flips · epoch ${market.nextOffer.epoch}` : 'The queue is empty';
      const meta = document.createElement('div'); meta.className = 'meta';
      for (const item of [`block ${market.blockHeight}`, `bundle ${short(market.nextOffer?.offerId)}`, `identity ${short(market.activeIdentity)}`, `code ${short(market.codeHash)}`]) {
        const span = document.createElement('span'); span.textContent = item; meta.append(span);
      }
      state.append(price, quoteLabel, title, meta);
      void refreshPreview(market.nextOffer);
      const ordersBox = $('open-orders'); ordersBox.replaceChildren();
      const mine = (market.openOrders || []).filter((order) =>
        order.buyer === market.activeIdentity || order.author === market.activeIdentity);
      if (mine.length) {
        const heading = document.createElement('h3'); heading.textContent = 'Your open orders'; ordersBox.append(heading);
        for (const order of mine.slice(0, 10)) {
          const button = document.createElement('button'); button.type = 'button';
          const seller = order.author === market.activeIdentity;
          button.textContent = `Order ${order.orderId} · ${order.state} · ${seller ? 'sell' : 'buy'} · ${short(order.offerId)}`;
          button.addEventListener('click', () => {
            const form = $(seller ? 'dm-form' : 'receive-form');
            form.querySelector('[name="orderId"]').value = order.orderId;
            form.scrollIntoView({behavior: 'smooth', block: 'center'});
          });
          ordersBox.append(button);
        }
      }
      if (market.nextOffer && ['buy', 'buyPublicSeed'].includes($('operation').value)) {
        $('operation').value = market.nextOffer.publicSeed ? 'buyPublicSeed' : 'buy';
        $('offer-id').value = market.nextOffer.offerId;
        updateFields();
      }
    } catch (error) { market = null; state.textContent = `Node state unavailable: ${error.message}`; $('market-preview').replaceChildren(); $('open-orders').replaceChildren(); }
  };
  const loadActions = async () => {
    try {
      const list = await request('/api/actions');
      const box = $('actions'); box.replaceChildren();
      if (!list.length) { box.textContent = 'No saved actions.'; return; }
      for (const action of list.reverse()) {
        const button = document.createElement('button'); button.type = 'button';
        button.textContent = `${action.operation} · ${action.preparedAt} · ${action.submission?.included ? (action.submission.success ? 'included' : 'failed') : action.attempted ? 'needs reconciliation' : 'unsigned'}`;
        button.addEventListener('click', () => showReview(action)); box.append(button);
      }
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
      void refreshMarket(); void loadActions();
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
      showReview(action); setMessage('Unsigned action prepared. Review the destination, amount, suggested fee ceiling, identity, and disclosure effect before signing.');
      await loadActions();
    } catch (error) { setMessage(error.message); }
    finally { form.querySelector('[name="passphrase"]').value = ''; $('prepare').disabled = false; }
  });
  $('submit').addEventListener('click', async () => {
    if (!selected || selected.attempted) return;
    if (!window.confirm('Sign and broadcast this reviewed action with real iDNA? Check the contract, amount, fee, and identity shown above.')) return;
    $('submit').disabled = true; setMessage('Signing and sending. Do not retry if the result is uncertain.');
    try {
      const action = await request('/api/submit', {method:'POST', headers:{'Content-Type':'application/json'},
        body:JSON.stringify({id:selected.id, reviewHash:selected.reviewHash})});
      showReview(action); setMessage(action.submission?.included ? 'Transaction included. Check the contract result before continuing.' : 'Sent; inclusion is not yet confirmed. Reconcile the saved hash before any retry.');
    } catch (error) { setMessage(error.message); }
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
    } catch (error) { setMessage(error.message); }
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
      updateFields();
      setMessage('Delivery verified. Review the confirm action separately before signing.');
    } catch (error) { setMessage(error.message); }
    finally { form.querySelector('[name="passphrase"]').value = ''; button.disabled = false; }
  });
  $('refresh').addEventListener('click', refreshMarket);
  $('reload-actions').addEventListener('click', loadActions);
  $('operation').addEventListener('change', () => {
    if (market?.nextOffer && ['buy', 'buyPublicSeed'].includes($('operation').value)) {
      $('operation').value = market.nextOffer.publicSeed ? 'buyPublicSeed' : 'buy';
      $('offer-id').value = market.nextOffer.offerId;
    }
    updateFields();
  });
  function updateFields() {
    const visible = new Set(actionFields[$('operation').value] || []);
    for (const label of $('prepare-form').querySelectorAll('[data-for]')) {
      const show = visible.has(label.dataset.for);
      label.hidden = !show;
      label.querySelector('input').disabled = !show;
    }
    $('evidence').hidden = !['session', 'proof', 'registration-host', 'vault', 'delivery'].some((field) => visible.has(field));
  }
  updateFields();
})();
