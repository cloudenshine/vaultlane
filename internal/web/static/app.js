/* ============== Vaultlane · 码仓 客户端 SPA ============== */
(() => {
'use strict';

// ---------- 启动前置：拉取公开配置（上游 base URL 等） ----------
window.CFG = window.CFG || { upstream_base: '' };
(async () => {
  try {
    const r = await fetch('/api/public/config', { headers: { 'Accept': 'application/json' } });
    if (r.ok) {
      const j = await r.json();
      if (j && j.data) window.CFG = Object.assign(window.CFG, j.data);
    }
  } catch (e) { /* 静默失败，前端用空 base 即可 */ }
})();

// ---------- API 封装 ----------
const API = {
  async get(path, params) {
    const url = params ? path + '?' + new URLSearchParams(params) : path;
    const r = await fetch(url, { headers: { 'Accept': 'application/json' } });
    return await r.json();
  },
  async post(path, body) {
    const r = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
      body: JSON.stringify(body || {}),
    });
    return await r.json();
  },
  async admin(path, opts = {}) {
    const token = localStorage.getItem('admin_token') || '';
    const r = await fetch(path, {
      method: opts.method || 'GET',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: opts.body ? JSON.stringify(opts.body) : undefined,
    });
    return await r.json();
  },
};

// ---------- 状态管理 ----------
const State = {
  categories: [],
  commodities: [],
  total: 0,
  page: 1,
  limit: 24,
  currentCat: 0,
  keywords: '',
  adminToken: localStorage.getItem('admin_token') || '',
  // v2.0.1 抽屉 / 下单状态
  drawerCommodity: null,
  drawerQty: 1,
  drawerSpec: null,
  orderableCache: null,
};

// ---------- 工具 ----------
const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

function formatPrice(p) {
  if (typeof p !== 'number') p = Number(p) || 0;
  const parts = p.toFixed(2).split('.');
  parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  return { int: parts[0], dec: parts[1] };
}

function escapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/[&<>"']/g, m => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  })[m]);
}

function toast(msg, type = '') {
  const t = $('#toast');
  t.textContent = msg;
  t.className = 'toast ' + (type ? 'toast-' + type : '');
  t.hidden = false;
  clearTimeout(t._timer);
  t._timer = setTimeout(() => { t.hidden = true; }, 2400);
}

// ---------- 视图：首页（商品网格） ----------
async function renderHome() {
  const main = $('#main');
  main.innerHTML = '<div class="loading">加载中…</div>';

  // 加载分类
  if (State.categories.length === 0) {
    const resp = await API.get('/api/categories');
    if (resp.code === 200) State.categories = resp.data;
  }

  // 加载商品
  const params = { page: State.page, limit: State.limit };
  if (State.currentCat > 0) params.category_id = State.currentCat;
  if (State.keywords) params.keywords = State.keywords;

  const resp = await API.get('/api/commodities', params);
  State.commodities = resp.data || [];
  State.total = resp.total || 0;

  main.innerHTML = `
    <div class="layout">
      ${renderSidebar()}
      <div class="main">
        <div class="section-title">
          <h2>${State.currentCat ? findCatName(State.currentCat) : '全部商品'}</h2>
          <span class="count">共 ${State.total} 件</span>
          <div class="actions">
            <input type="number" id="page-jump" value="${State.page}" min="1" style="width:70px" />
            <button class="btn btn-secondary" style="flex:none;padding:8px 14px" onclick="window.Mastore.goPage()">跳转</button>
          </div>
        </div>
        ${renderGoodsGrid()}
      </div>
    </div>
  `;
}

function findCatName(id) {
  for (const c of State.categories) {
    if (c.id === id) return c.name;
    if (c.children) {
      for (const sc of c.children) if (sc.id === id) return sc.name;
    }
  }
  return '分类';
}

function renderSidebar() {
  const items = State.categories.map(c => {
    const sub = c.children || [];
    return `
      <li>
        <div class="cat-item ${State.currentCat === c.id ? 'active' : ''}" onclick="Mastore.selectCat(${c.id})">
          <span class="cat-icon">${c.icon ? `<img src="${c.icon}" alt="" onerror="this.style.display='none'" />` : escapeHtml(c.name[0])}</span>
          <span class="cat-name">${escapeHtml(c.name)}</span>
          <span class="cat-count">${c.commodity_count || 0}</span>
        </div>
        ${sub.length ? `<ul class="subcat-list">${sub.map(sc => `
          <li class="subcat-item ${State.currentCat === sc.id ? 'active' : ''}" onclick="Mastore.selectCat(${sc.id})">${escapeHtml(sc.name)} · ${sc.commodity_count || 0}</li>
        `).join('')}</ul>` : ''}
      </li>
    `;
  }).join('');
  return `
    <aside class="sidebar">
      <div class="sidebar-title">商品分类</div>
      <ul class="cat-list">
        <li class="cat-all" onclick="Mastore.selectCat(0)">⌂ 全部商品</li>
        ${items}
      </ul>
    </aside>
  `;
}

function renderGoodsGrid() {
  if (State.commodities.length === 0) {
    return `<div class="empty-state"><div class="empty-state-icon">∅</div>暂无符合条件的商品</div>`;
  }
  return `<div class="goods-grid">${State.commodities.map(c => {
    const p = formatPrice(c.price);
    return `
      <div class="goods-card" onclick="Mastore.openDetail(${c.id})">
        <div class="goods-cover">
          ${c.cover ? `<img src="${c.cover}" alt="" loading="lazy" onerror="this.replaceWith(Object.assign(document.createElement('div'),{className:'goods-cover-fallback',textContent:'${escapeHtml((c.category_name||c.name||'').slice(0,1))}'}))" />` : `<div class="goods-cover-fallback">${escapeHtml((c.category_name||c.name||'').slice(0,1))}</div>`}
          ${c.recommend ? '<div class="goods-recommend">推荐</div>' : ''}
        </div>
        <div class="goods-body">
          <div class="goods-name">${escapeHtml(c.name)}</div>
          <div class="goods-tags">
            ${(c.tags || []).slice(0, 2).map(t => `<span class="tag">${escapeHtml(t)}</span>`).join('')}
            <span class="stock-badge" style="color:${c.stock_label.color}">${escapeHtml(c.stock_label.text)}</span>
            <span class="tag">${escapeHtml(c.delivery_label)}</span>
          </div>
          <div class="goods-footer">
            <div>
              <span class="goods-price"><span class="goods-price-prefix">¥</span>${p.int}<span class="goods-price-decimal">.${p.dec}</span></span>
              ${c.user_price && c.user_price !== c.price ? `<span style="color:var(--text-3);text-decoration:line-through;margin-left:6px;font-size:11px">¥${c.user_price.toFixed(2)}</span>` : ''}
            </div>
            <div class="goods-sold">已售 ${c.order_sold || 0}</div>
          </div>
        </div>
      </div>
    `;
  }).join('')}</div>`;
}

// ---------- 视图：商品详情抽屉 ----------
async function openDetail(id) {
  // v2.0：详情改全屏独立路由
  // 显式调 renderRoute（不依赖 hashchange，兼容 controlled browser）
  location.hash = '#/commodity/' + id;
  renderRoute();
}

// ---------- 视图：订单查询 ----------
async function renderOrders() {
  const main = $('#main');
  main.innerHTML = `
    <div class="layout-2col">
      <div class="card">
        <div class="card-head"><h2>可下单商品</h2></div>
        <div id="orderable-grid"><div class="loading">加载中…</div></div>
      </div>
      <div class="card">
        <div class="card-head"><h2>查询订单</h2></div>
        <div class="form">
          <div class="field"><label>订单号</label><input id="order-no" placeholder="输入订单号" /></div>
          <button class="btn btn-primary" onclick="Mastore.lookupOrder()">查询</button>
        </div>
        <div id="order-result" style="margin-top:16px"></div>
      </div>
    </div>
  `;
  await Mastore.loadOrderable();
}

// ---------- 路由 ----------
function renderRoute() {
  const hash = location.hash || '#/';
  const parts = hash.replace(/^#\//, '').split('/');
  const route = parts[0] || '';
  const arg = parts[1] || '';
  const arg2 = parts[2] || '';

  // 顶栏 active
  $$('.nav-link').forEach(a => {
    a.classList.toggle('active', a.dataset.route === route || (route === '' && a.dataset.route === 'home'));
  });

  // 顶栏登录态
  renderTopbarUser();

  if (route === '' || route === 'home') {
    renderHome();
  } else if (route === 'orders') {
    renderOrders();
  } else if (route === 'commodity') {
    renderCommodityDetail(arg);
  } else if (route === 'order') {
    renderOrderPage(arg);
  } else if (route === 'login') {
    renderLoginPage();
  } else if (route === 'register') {
    renderRegisterPage();
  } else if (route === 'profile') {
    renderProfilePage();
  } else if (route === 'checkout') {
    renderCheckoutPage(arg);
  } else {
    renderHome();
  }
}

// ---------- 顶栏登录态 ----------
async function renderTopbarUser() {
  const nav = $('.topbar-nav');
  if (!nav) return;
  // 简化：读 localStorage 缓存
  let u = null;
  try { u = JSON.parse(localStorage.getItem('mastore_user') || 'null'); } catch (e) {}
  // 已有 login/profile 链接则不动
  if (nav.querySelector('#nav-profile')) return;
  const loginLink = document.createElement('a');
  loginLink.id = 'nav-profile';
  loginLink.className = 'nav-link';
  if (u) {
    loginLink.href = '#/profile';
    loginLink.textContent = u.username || '我的';
  } else {
    loginLink.href = '#/login';
    loginLink.textContent = '登录/注册';
  }
  nav.appendChild(loginLink);
}

async function fetchMe() {
  try {
    const r = await apiGet('/api/user/profile');
    if (r.code === 200) { localStorage.setItem('mastore_user', JSON.stringify(r.data)); return r.data; }
  } catch (e) {}
  localStorage.removeItem('mastore_user');
  return null;
}

// ---------- 登录页 ----------
function renderLoginPage() {
  const main = $('#main');
  main.innerHTML = `
    <div class="auth-page">
      <div class="card">
        <div class="card-head"><h2>登录</h2></div>
        <div class="form">
          <div class="field"><label>用户名/邮箱</label><input id="login-account" placeholder="username or email" /></div>
          <div class="field"><label>密码</label><input id="login-password" type="password" placeholder="密码" /></div>
          <div class="form-actions">
            <button class="btn btn-primary" onclick="Mastore.doLogin()">登录</button>
            <a class="btn" href="#/register">注册新账号</a>
          </div>
        </div>
      </div>
    </div>`;
  setTimeout(() => $('#login-account')?.focus(), 0);
}

async function doLogin() {
  const account = $('#login-account')?.value.trim();
  const password = $('#login-password')?.value;
  if (!account || !password) { toast('请填写账号和密码'); return; }
  const r = await apiPost('/api/user/login', { account, password });
  if (r.code !== 200) { toast(r.msg || '登录失败'); return; }
  localStorage.setItem('mastore_user', JSON.stringify(r.data));
  toast('登录成功');
  setTimeout(() => { location.hash = '#/profile'; }, 300);
}

// ---------- 注册页 ----------
function renderRegisterPage() {
  const main = $('#main');
  main.innerHTML = `
    <div class="auth-page">
      <div class="card">
        <div class="card-head"><h2>注册</h2></div>
        <div class="form">
          <div class="field"><label>用户名</label><input id="reg-username" placeholder="3-20 位字母/数字/下划线" /></div>
          <div class="field"><label>邮箱</label><input id="reg-email" type="email" placeholder="email@example.com" /></div>
          <div class="field"><label>密码</label><input id="reg-password" type="password" placeholder="6-64 位" /></div>
          <div class="field"><label>邀请码（可选）</label><input id="reg-invite" placeholder="INV0001ABC" /></div>
          <div class="form-actions">
            <button class="btn btn-primary" onclick="Mastore.doRegister()">注册</button>
            <a class="btn" href="#/login">已有账号？登录</a>
          </div>
        </div>
      </div>
    </div>`;
}

async function doRegister() {
  const username = $('#reg-username')?.value.trim();
  const email = $('#reg-email')?.value.trim();
  const password = $('#reg-password')?.value;
  const invite_code = $('#reg-invite')?.value.trim();
  if (!username || !email || !password) { toast('请完整填写'); return; }
  const r = await apiPost('/api/user/register', { username, email, password, invite_code });
  if (r.code !== 200) { toast(r.msg || '注册失败'); return; }
  localStorage.setItem('mastore_user', JSON.stringify(r.data));
  toast('注册成功');
  setTimeout(() => { location.hash = '#/profile'; }, 300);
}

// ---------- 个人中心 ----------
async function renderProfilePage() {
  const main = $('#main');
  main.innerHTML = '<div class="loading">加载中…</div>';
  const me = await fetchMe();
  if (!me) {
    main.innerHTML = `<div class="empty"><p>请先 <a href="#/login">登录</a></p></div>`;
    return;
  }
  const orders = (me.recent_orders || []).map(o => `
    <div class="profile-order">
      <a class="profile-order-no" href="#/order/${encodeURIComponent(o.trade_no)}">${escapeHtml(o.trade_no)}</a>
      <span>${escapeHtml(o.commodity_name||'#'+o.commodity_id)}</span>
      <span>¥${(o.amount||0).toFixed(2)}</span>
      <span class="badge">${orderStatusText(o.status)}</span>
    </div>
  `).join('') || '<div class="empty-sub">暂无订单</div>';
  main.innerHTML = `
    <div class="profile-page">
      <div class="card">
        <div class="card-head"><h2>个人中心</h2></div>
        <div class="profile-info">
          <div class="kv-row"><span class="kv-key">用户</span><span class="kv-val">${escapeHtml(me.username)}</span></div>
          <div class="kv-row"><span class="kv-key">邮箱</span><span class="kv-val">${escapeHtml(me.email||'—')}</span></div>
          <div class="kv-row"><span class="kv-key">余额</span><span class="kv-val">¥${(me.balance||0).toFixed(2)}</span></div>
          <div class="kv-row"><span class="kv-key">注册时间</span><span class="kv-val">${formatDate(me.created_at)}</span></div>
        </div>
        <div class="profile-actions">
          <a class="btn" href="#/orders">查看全部订单</a>
          <button class="btn" onclick="Mastore.doLogout()">退出登录</button>
        </div>
      </div>
      <div class="card" id="referral-card">
        <div class="card-head"><h2>邀请返佣</h2></div>
        <div class="profile-info" id="referral-box"><div class="empty-sub">加载中…</div></div>
      </div>
      <div class="card">
        <div class="card-head"><h2>最近订单</h2></div>
        <div class="profile-orders">${orders}</div>
      </div>
    </div>`;
  try {
    const ref = await apiGet('/api/user/referral');
    const box = document.getElementById('referral-box');
    if (box && ref.code === 200 && ref.data) {
      const d = ref.data;
      box.innerHTML = `
        <div class="kv-row"><span class="kv-key">邀请码</span><span class="kv-val"><code>${escapeHtml(d.invite_code||'')}</code></span></div>
        <div class="kv-row"><span class="kv-key">邀请人数</span><span class="kv-val">${d.total_invites||0}</span></div>
        <div class="kv-row"><span class="kv-key">累计返佣</span><span class="kv-val">¥${(d.total_commission||0).toFixed(2)}</span></div>
        <div class="kv-row"><span class="kv-key">佣金余额</span><span class="kv-val">¥${(d.commission_balance||0).toFixed(2)}</span></div>`;
    } else if (box) {
      box.innerHTML = '<div class="empty-sub">暂无邀请数据</div>';
    }
  } catch (e) {}
}

async function doLogout() {
  await apiPost('/api/user/logout', {});
  localStorage.removeItem('mastore_user');
  toast('已退出');
  setTimeout(() => { location.hash = '#/'; }, 300);
}

// ---------- 支付页 ----------
async function renderCheckoutPage(orderId) {
  const main = $('#main');
  main.innerHTML = '<div class="loading">加载中…</div>';
  if (!orderId) { location.hash = '#/'; return; }
  // 通过 ListOrders 反查（极简）
  let order = null;
  try {
    const orders = await apiGet('/api/payment/methods');
    // 拿到 method 列表
  } catch (e) {}
  // 直接拿订单
  try {
    const r = await apiGet('/api/orders/' + orderId);
    if (r.code === 200) order = r.data.order;
  } catch (e) {}
  if (!order) { main.innerHTML = `<div class="error">订单不存在</div>`; return; }
  if (order.status >= 1) { location.hash = '#/order/' + order.trade_no; return; }
  let methods = [];
  try {
    const r = await apiGet('/api/payment/methods');
    methods = r.data || [];
  } catch (e) {}
  // 余额判断
  let me = await fetchMe();
  main.innerHTML = `
    <div class="checkout-page">
      <a class="back-link" href="#/">← 返回</a>
      <h1>支付订单</h1>
      <div class="card">
        <div class="kv-row"><span class="kv-key">订单号</span><span class="kv-val"><code>${escapeHtml(order.trade_no)}</code></span></div>
        <div class="kv-row"><span class="kv-key">商品</span><span class="kv-val">${escapeHtml(order.commodity_name||'#'+order.commodity_id)}</span></div>
        <div class="kv-row"><span class="kv-key">数量</span><span class="kv-val">${order.num}</span></div>
        <div class="kv-row"><span class="kv-key">金额</span><span class="kv-val"><strong class="price">¥${(order.amount||0).toFixed(2)}</strong></span></div>
      </div>
      <div class="card">
        <div class="card-head"><h2>选择支付方式</h2></div>
        <div class="pay-methods">
          ${methods.map(m => `
            <label class="pay-method">
              <input type="radio" name="pay-method" value="${escapeHtml(m)}" ${m==='balance'?'checked':''} />
              <span>${payMethodLabel(m)}${m==='balance' && me ? '（余额 ¥'+(me.balance||0).toFixed(2)+'）' : ''}</span>
            </label>
          `).join('') || '<div class="empty-sub">暂无可用支付方式</div>'}
        </div>
        <div class="form-actions">
          <button class="btn btn-primary" onclick="Mastore.doPay(${order.id})">立即支付</button>
        </div>
      </div>
    </div>`;
}

function payMethodLabel(m) {
  return {
    balance: '余额支付',
    alipay: '支付宝',
    wxpay: '微信支付',
    usdt: 'USDT (TRC20)',
    epay: '易支付',
  }[m] || m;
}

async function doPay(orderId) {
  const m = document.querySelector('input[name="pay-method"]:checked')?.value;
  if (!m) { toast('请选择支付方式'); return; }
  if (m === 'balance') {
    // 余额支付直接走 /api/orders?  不，余额支付必须先有订单（已创建），调 /api/payment/callback/balance
    // 这里通过 /api/orders 创建带 pay_method=balance 的订单（同 trade_no 是新的）
    // 简化做法：调 /api/payment/callback/balance 内部触发
    const r = await apiPost('/api/payment/balance/finish', { order_id: orderId });
    if (r.code !== 200) { toast(r.msg || '支付失败'); return; }
    toast('支付成功');
    // 取回订单
    const list = await apiGet('/api/orders/items/list'); // 实际是去重
    setTimeout(() => { location.hash = '#/'; }, 500);
    return;
  }
  // 其它渠道走 create
  const r = await apiPost('/api/payment/create', { order_id: orderId, method: m });
  if (r.code !== 200) { toast(r.msg || '创建支付失败'); return; }
  if (r.data.form_html) {
    // 渲染表单并自动提交
    const div = document.createElement('div');
    div.innerHTML = r.data.form_html;
    document.body.appendChild(div);
    return;
  }
  if (r.data.pay_url) {
    if (/^https?:\/\//.test(r.data.pay_url)) {
      location.href = r.data.pay_url;
    } else {
      toast('请手动完成支付，订单号：' + orderId);
    }
  }
}

// 工具：日期格式化
function formatDate(s) {
  if (!s) return '—';
  try { return new Date(s).toLocaleString('zh-CN'); } catch (e) { return s; }
}

async function renderCommodityDetail(id) {
  if (!id) { location.hash = '#/'; return; }
  const main = $('#main');
  main.innerHTML = '<div class="loading">加载中…</div>';
  try {
    const r = await apiGet('/api/commodities/' + id);
    if (r.code !== 200) { main.innerHTML = `<div class="error">${escapeHtml(r.msg||'加载失败')}</div>`; return; }
    const d = r.data;
    const c = d.commodity;
    const desc = d.description || '暂无描述';
    // description 是上游 HTML 字符串：直接作为 HTML 渲染（前端可信源已设 CSP 限制）
    // 内嵌 <img> 会在 frontend.js 的 rebindImages 里补全 base URL
    const specHtml = renderSpecList(d.config, c.price);
    const minQty = Math.max(1, d.minimum || 1);
    const maxQty = d.maximum > 0 ? d.maximum : c.stock;
    main.innerHTML = `
      <article class="detail-page">
        <a class="back-link" href="#/">← 返回商品列表</a>
        <div class="detail-hero">
          <img class="detail-cover" src="${escapeHtml(c.cover)}" alt="${escapeHtml(c.name)}" onerror="this.style.display='none'" />
          <div class="detail-info">
            <h1>${escapeHtml(c.name)}</h1>
            <div class="detail-price-row">
              <span class="detail-price">¥${(c.price||0).toFixed(2)}</span>
              <span class="badge ${c.stock>0?'badge-ok':'badge-err'}">${c.stock_label?.text||'有货'}</span>
            </div>
            <div class="detail-tags">${(c.tags||[]).map(t=>`<span class="tag">${escapeHtml(t)}</span>`).join('')}</div>
            <div class="detail-stats">已售 ${c.order_sold||0} · 库存 ${c.stock} · ${escapeHtml(c.delivery_label||'自动发货')}</div>
            <div class="detail-actions">
              <button class="btn btn-primary" onclick="Mastore.openOrderable('${c.id}')">立即下单</button>
              <a class="btn" href="#/">继续逛</a>
            </div>
          </div>
        </div>
        ${specHtml ? `<section class="detail-section"><h2>规格选择</h2>${specHtml}</section>` : ''}
        <section class="detail-section">
          <h2>商品详情</h2>
          <div class="detail-content" id="detail-content-html"></div>
        </section>
        ${d.password_status ? `<section class="detail-section"><h2>密码状态</h2><p>${d.password_status===1?'有密码':'无密码'}</p></section>` : ''}
        ${d.share_url ? `<section class="detail-section"><h2>分享链接</h2><input type="text" readonly value="${escapeHtml(d.share_url)}" /></section>` : ''}
        <section class="detail-section">
          <h2>购买提示</h2>
          <ul class="detail-tips">
            <li>起购：${minQty} 件</li>
            <li>限购：${maxQty} 件</li>
            <li>联系方式：${d.contact_type===0?'邮箱':'手机'}</li>
            <li>发货方式：${d.delivery_way===1?'自动发货':'手动发货'}</li>
          </ul>
        </section>
      </article>
    `;
    // description 当 HTML 渲染 + 内嵌 img 路径补全
    const descEl = document.getElementById('detail-content-html');
    descEl.innerHTML = desc;
    rebindImages(descEl);
    rebindSpecSelector();
    window.scrollTo({ top: 0 });
  } catch (e) {
    main.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

// 把容器内 <img> 的相对路径补全为绝对 URL
function rebindImages(root) {
  if (!root) return;
  const base = (window.CFG && window.CFG.upstream_base) || '';
  root.querySelectorAll('img').forEach(img => {
    const src = img.getAttribute('src') || '';
    if (!src || /^https?:\/\//i.test(src) || src.startsWith('data:')) return;
    if (base && src.startsWith('/')) img.src = base + src;
    img.style.maxWidth = '100%';
    img.style.height = 'auto';
    img.loading = 'lazy';
  });
}

// 解析 upstream config → 规格列表
// 上游 config 形如：{category:{"日卡":1.0,"周卡":5.0}, category_agent_price:{...}}
// 或旧格式：{spec: [{name,price}]} 或 "日卡|1.0,周卡|5.0"
function renderSpecList(raw, defaultPrice) {
  if (!raw) return '';
  let cfg = raw;
  if (typeof raw === 'string') {
    try { cfg = JSON.parse(raw); } catch (e) { return ''; }
  }
  if (typeof cfg !== 'object' || cfg === null) return '';

  // 形如 {category: {...}, category_agent_price: {...}}
  const category = cfg.category || cfg.spec;
  if (category && typeof category === 'object' && !Array.isArray(category)) {
    const items = Object.keys(category).map(k => ({
      name: k,
      price: parseFloat(category[k]) || defaultPrice,
    }));
    if (items.length) return renderSpecItems(items, defaultPrice);
  }
  // 形如 [{name, price}, ...]
  if (Array.isArray(cfg)) {
    const items = cfg.map(x => ({
      name: x.name || x.spec || x.label || '?',
      price: parseFloat(x.price ?? x.value) || defaultPrice,
    }));
    if (items.length) return renderSpecItems(items, defaultPrice);
  }
  return '';
}

function renderSpecItems(items, defaultPrice) {
  return `
    <div class="spec-list" data-spec>
      ${items.map((it, i) => `
        <div class="spec-item${i===0?' selected':''}" data-name="${escapeHtml(it.name)}" data-price="${it.price}">
          <span class="spec-name">${escapeHtml(it.name)}</span>
          <span class="spec-price">¥${it.price.toFixed(2)}</span>
        </div>
      `).join('')}
    </div>
    <input type="hidden" id="selected-spec-name" value="${escapeHtml(items[0].name)}" />
    <input type="hidden" id="selected-spec-price" value="${items[0].price}" />
  `;
}

function rebindSpecSelector() {
  const list = document.querySelector('[data-spec]');
  if (!list) return;
  list.querySelectorAll('.spec-item').forEach(el => {
    el.onclick = () => {
      list.querySelectorAll('.spec-item').forEach(x => x.classList.remove('selected'));
      el.classList.add('selected');
      const nameEl = document.getElementById('selected-spec-name');
      const priceEl = document.getElementById('selected-spec-price');
      if (nameEl) nameEl.value = el.dataset.name;
      if (priceEl) priceEl.value = el.dataset.price;
    };
  });
}

async function renderOrderPage(tradeNo) {
  if (!tradeNo) { location.hash = '#/orders'; return; }
  const main = $('#main');
  main.innerHTML = '<div class="loading">加载中…</div>';
  try {
    const r = await apiGet('/api/orders/' + tradeNo);
    if (r.code !== 200) { main.innerHTML = `<div class="error">${escapeHtml(r.msg)}</div>`; return; }
    const o = r.data;
    const secrets = r.secrets || [];
    main.innerHTML = `
      <article class="order-page">
        <a class="back-link" href="#/">← 返回首页</a>
        <h1>订单详情</h1>
        <div class="kv">
          <div class="kv-row"><span class="kv-key">订单号</span><span class="kv-val"><code>${o.trade_no}</code></span></div>
          <div class="kv-row"><span class="kv-key">商品</span><span class="kv-val">${escapeHtml(o.commodity_name||'#'+o.commodity_id)}</span></div>
          <div class="kv-row"><span class="kv-key">金额</span><span class="kv-val">¥${(o.amount||0).toFixed(2)}</span></div>
          <div class="kv-row"><span class="kv-key">状态</span><span class="kv-val">${orderStatusText(o.status)}</span></div>
          <div class="kv-row"><span class="kv-key">联系方式</span><span class="kv-val">${escapeHtml(o.contact||'—')}</span></div>
          <div class="kv-row"><span class="kv-key">来源</span><span class="kv-val">${escapeHtml(o.source||'—')}</span></div>
        </div>
        ${secrets.length ? `
          <h2>卡密</h2>
          <div class="secrets-list">
            ${secrets.map((s,i) => `
              <div class="secret-item">
                <code>${escapeHtml(s)}</code>
                <button class="btn btn-sm" onclick="Mastore.copySecret('${escapeHtml(s.replace(/'/g,"\\'"))}')">复制</button>
              </div>
            `).join('')}
          </div>
        ` : ''}
      </article>
    `;
  } catch (e) {
    main.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

function orderStatusText(st) {
  return ['待支付','已支付','已发货','已退款'][st] || '—';
}

function selectCat(id) {
  State.currentCat = id;
  State.page = 1;
  renderHome();
}

function goPage() {
  const v = Number($('#page-jump').value) || 1;
  State.page = Math.max(1, v);
  renderHome();
}

function doSearch() {
  State.keywords = $('#search-input').value.trim();
  State.currentCat = 0;
  State.page = 1;
  renderHome();
}

// ---------- 启动 ----------
window.Mastore = {
  // 路由 / 视图
  selectCat, goPage, doSearch,
  // 详情 / 订单
  openDetail, renderCommodityDetail, renderOrderPage, lookupOrder, reopenDetail,
  // 抽屉
  openDrawer, closeDrawer, openOrderable, openOrderableDrawer, selectSpec, changeQty,
  // 下单
  submitOrder, submitOrderByCode, openConfirm, closeConfirm,
  // 订单页 / 卡密
  loadOrderable, copySecret, gotoOrder,
  // 用户 / 支付
  doLogin, doRegister, doLogout, doPay, fetchMe,
  renderLoginPage, renderRegisterPage, renderProfilePage, renderCheckoutPage,
  // 工具
  apiGet, apiPost, escapeHtml, toast,
};

try {
  document.addEventListener('hashchange', () => {
    try { renderRoute(); } catch (e) { console.error('[renderRoute err]', e); }
  });
} catch (e) {
  console.error('[init] addEventListener failed:', e);
}
document.addEventListener('DOMContentLoaded', () => {
  $('#search-btn')?.addEventListener('click', doSearch);
  $('#search-input')?.addEventListener('keydown', e => { if (e.key === 'Enter') doSearch(); });

  // v2.0.1 防回归：closeDrawer 监听改成事件代理 + 命名空间
  // 即使 closeDrawer 因代码错误未定义，也不会在启动时崩溃整个脚本
  document.addEventListener('click', (e) => {
    if (e.target.matches?.('.drawer-mask')) safeCloseDrawer();
    if (e.target.matches?.('#confirm-cancel')) closeConfirm(false);
    if (e.target.matches?.('#confirm-ok')) closeConfirm(true);
    if (e.target.matches?.('.modal-mask')) closeConfirm(false);
    if (e.target.matches?.('#lookup-btn')) lookupOrder();
  });

  // 确认弹窗按钮绑定（直接 addEventListener，避免 onclick 属性依赖 window.Mastore）
  $('#confirm-ok')?.addEventListener('click', () => closeConfirm(true));
  $('#confirm-cancel')?.addEventListener('click', () => closeConfirm(false));

  renderRoute();
});

function safeCloseDrawer() {
  try { closeDrawer(); }
  catch (e) { console.warn('closeDrawer failed:', e); }
}

// ============== v2.0.1 恢复区 ==============
// 以下函数在上一轮误删，此处统一恢复。**请勿再次删除**，否则前端抽屉/下单流程全部瘫痪。

// ---------- 通用 ----------
async function apiGet(path, params) {
  const url = params ? path + '?' + new URLSearchParams(params) : path;
  const r = await fetch(url, { headers: { 'Accept': 'application/json' }, credentials: 'same-origin' });
  return await r.json();
}

async function apiPost(path, body) {
  const r = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify(body || {}),
  });
  return await r.json();
}

// ---------- 二次确认弹窗（Promise 化）----------
let _confirmResolver = null;
function openConfirm(title, msg) {
  return new Promise(resolve => {
    _confirmResolver = resolve;
    $('#confirm-title').textContent = title || '确认操作';
    $('#confirm-msg').textContent = msg || '是否继续？';
    $('#confirm-modal').hidden = false;
  });
}
function closeConfirm(ok) {
  $('#confirm-modal').hidden = true;
  if (_confirmResolver) {
    _confirmResolver(!!ok);
    _confirmResolver = null;
  }
}

// ---------- 抽屉 ----------
function openDrawer(title, bodyHtml) {
  const panel = $('#drawer-panel');
  panel.innerHTML = `
    <header class="drawer-header">
      <h3 class="drawer-title">${escapeHtml(title || '')}</h3>
      <button class="drawer-close" type="button" aria-label="关闭">×</button>
    </header>
    <div class="drawer-body">${bodyHtml || ''}</div>
  `;
  panel.querySelector('.drawer-close')?.addEventListener('click', closeDrawer);
  $('#drawer').hidden = false;
}

function closeDrawer() {
  $('#drawer').hidden = true;
  $('#drawer-panel').innerHTML = '';
  State.drawerCommodity = null;
  State.drawerQty = 1;
  State.drawerSpec = null;
}

// ---------- 抽屉状态（见 State 对象初始化）----------

function changeQty(delta) {
  const input = $('#qty-input');
  if (!input) return;
  let v = parseInt(input.value || '1', 10);
  if (delta === 0) {
    // 直接输入
    v = Math.max(1, isNaN(v) ? 1 : v);
  } else {
    v = Math.max(1, v + delta);
  }
  const max = State.drawerCommodity?.stock || 99;
  if (v > max) v = max;
  input.value = v;
  State.drawerQty = v;
  updateDrawerTotal();
}

function selectSpec(specRace, specName, specPrice) {
  State.drawerSpec = { race: specRace, name: specName, price: specPrice };
  document.querySelectorAll('.spec-item').forEach(el => {
    el.classList.toggle('selected', el.dataset.race === specRace);
  });
  State.drawerCommodity.price = specPrice;  // 规格价格覆盖
  $('#drawer-price').textContent = '¥' + specPrice.toFixed(2);
  updateDrawerTotal();
}

function updateDrawerTotal() {
  const c = State.drawerCommodity;
  if (!c) return;
  const unit = State.drawerSpec?.price ?? c.price ?? 0;
  const total = unit * State.drawerQty;
  const el = $('#drawer-total');
  if (el) el.textContent = '¥' + total.toFixed(2);
}

// ---------- 立即下单入口 ----------
async function openOrderable(commodityId) {
  // 通过商品 ID（兼容自营 + 上游）从列表缓存取详情
  // 优先用 /api/commodities/:id 拿最新数据
  let detail = null;
  try {
    const r = await apiGet('/api/commodities/' + commodityId);
    if (r.code === 200) detail = r.data;
  } catch (e) { /* ignore */ }
  if (!detail) {
    toast('加载商品详情失败', 'err');
    return;
  }
  openOrderableDrawer(detail);
}

function openOrderableDrawer(detail) {
  // detail: { commodity: {...}, description, config, minimum, maximum, password_status, delivery_way }
  const c = detail.commodity;
  State.drawerCommodity = {
    id: c.id,
    name: c.name,
    price: c.price,
    stock: c.stock,
    delivery_way: detail.delivery_way ?? 1,
    password_status: detail.password_status ?? 0,
    source: c.source || 'upstream:upstreama',
    shared_code: c.shared_code || '',
  };
  State.drawerQty = 1;
  State.drawerSpec = null;

  const max = c.stock > 0 ? c.stock : (detail.maximum || 99);
  const min = detail.minimum || 1;

  const body = `
    <div class="drawer-cover">
      <img src="${escapeHtml(c.cover)}" alt="${escapeHtml(c.name)}" onerror="this.style.display='none'" />
    </div>
    <div class="drawer-price-row">
      <span id="drawer-price" class="drawer-price">¥${(c.price || 0).toFixed(2)}</span>
      <span class="drawer-stock">库存 ${c.stock} · ${escapeHtml(c.delivery_label || '自动发货')}</span>
    </div>

    ${detail.config ? `<div class="drawer-section">
      <div class="drawer-section-title">规格说明</div>
      <pre style="white-space:pre-wrap;font-size:12px;color:var(--text-2);background:var(--surface-2);padding:10px;border-radius:6px;max-height:120px;overflow:auto">${escapeHtml(detail.config)}</pre>
    </div>` : ''}

    <div class="drawer-section">
      <div class="drawer-section-title">购买数量</div>
      <div class="qty-stepper">
        <button type="button" id="qty-dec" aria-label="减少">−</button>
        <input id="qty-input" type="number" min="1" max="${max}" value="1" />
        <button type="button" id="qty-inc" aria-label="增加">+</button>
        <span style="font-size:11px;color:var(--text-3);margin-left:8px">最少 ${min} 件</span>
      </div>
    </div>

    <div class="drawer-field">
      <label>联系方式（邮箱 / 手机号）</label>
      <input id="contact-input" type="text" placeholder="your@email.com 或 138xxxxxxxx" />
    </div>
    <div class="drawer-field">
      <label>优惠券码（可选）</label>
      <input id="coupon-input" type="text" placeholder="如 NEW10" />
    </div>

    ${State.drawerCommodity.password_status ? `<div class="drawer-field">
      <label>查询密码（取卡密时需要）</label>
      <input id="password-input" type="text" placeholder="可留空" />
    </div>` : ''}

    <div class="drawer-total">
      <span class="drawer-total-label">合计</span>
      <span id="drawer-total" class="drawer-total-price">¥${(c.price || 0).toFixed(2)}</span>
    </div>

    <button id="submit-order-btn" class="btn btn-primary btn-block" type="button">立即下单</button>
  `;
  openDrawer(c.name, body);

  // 事件绑定
  $('#qty-inc')?.addEventListener('click', () => changeQty(1));
  $('#qty-dec')?.addEventListener('click', () => changeQty(-1));
  $('#qty-input')?.addEventListener('input', () => changeQty(0));
  $('#submit-order-btn')?.addEventListener('click', () => {
    if (State.drawerCommodity.source === 'self') submitOrder();
    else submitOrderByCode();
  });
  updateDrawerTotal();
}

// ---------- 提交订单 ----------
async function submitOrder() {
  // 自营 / 上游 /shared/commodity/items 兼容
  await submitOrderByCode();
}

async function submitOrderByCode() {
  const c = State.drawerCommodity;
  if (!c) return;
  const contact = $('#contact-input')?.value.trim() || '';
  if (!contact) {
    toast('请填写联系方式', 'warn');
    $('#contact-input')?.focus();
    return;
  }
  const password = $('#password-input')?.value || '';
  const qty = State.drawerQty;

  const ok = await openConfirm(
    '确认下单',
    `商品：${c.name}\n数量：${qty}\n合计：¥${(c.price * qty).toFixed(2)}\n联系方式：${contact}\n\n确认提交？`
  );
  if (!ok) return;

  const btn = $('#submit-order-btn');
  btn.disabled = true;
  btn.textContent = '提交中…';

  try {
    const couponCode = $('#coupon-input')?.value.trim() || '';
    const body = {
      contact,
      num: qty,
      password,
      pay_method: 'balance',
    };
    if (couponCode) body.coupon_code = couponCode;
    if (c.shared_code) {
      body.shared_code = c.shared_code;
      if (State.drawerSpec?.race) body.race = State.drawerSpec.race;
    } else {
      body.commodity_id = c.id;
    }

    const r = await API.post('/api/orders', body);
    btn.disabled = false;
    btn.textContent = '立即下单';

    if (r.code !== 200) {
      toast(r.msg || '下单失败', 'err');
      return;
    }
    toast('下单成功！', 'ok');
    closeDrawer();
    // v3.0 流程：需要支付则跳支付页；否则跳订单详情
    if (r.data && r.data.need_pay) {
      const orderId = r.data.order?.id || r.order?.id;
      if (orderId) {
        setTimeout(() => { location.hash = '#/checkout/' + orderId; }, 300);
        return;
      }
    }
    const tradeNo = r.data?.trade_no || r.trade?.trade_no || '';
    if (tradeNo) {
      setTimeout(() => gotoOrder(tradeNo), 300);
    } else {
      console.warn('No trade_no in response', r);
    }
  } catch (e) {
    btn.disabled = false;
    btn.textContent = '立即下单';
    toast('下单异常: ' + e.message, 'err');
  }
}

// ---------- 订单查询 ----------
async function lookupOrder() {
  const tradeNo = $('#order-no')?.value.trim();
  if (!tradeNo) {
    toast('请输入订单号', 'warn');
    return;
  }
  const out = $('#order-result');
  out.innerHTML = '<div class="loading">查询中…</div>';
  try {
    const r = await apiGet('/api/orders/' + encodeURIComponent(tradeNo));
    if (r.code !== 200) {
      out.innerHTML = `<div class="error">${escapeHtml(r.msg || '查询失败')}</div>`;
      return;
    }
    const o = r.data;
    const secrets = r.secrets || [];
    out.innerHTML = `
      <div class="card">
        <div class="card-head"><h3>订单 #${escapeHtml(o.trade_no)}</h3></div>
        <div class="kv">
          <div class="kv-row"><span class="kv-key">商品</span><span class="kv-val">${escapeHtml(o.commodity_name || ('#' + o.commodity_id))}</span></div>
          <div class="kv-row"><span class="kv-key">数量</span><span class="kv-val">${o.num || 1}</span></div>
          <div class="kv-row"><span class="kv-key">金额</span><span class="kv-val">¥${(o.amount || 0).toFixed(2)}</span></div>
          <div class="kv-row"><span class="kv-key">状态</span><span class="kv-val">${orderStatusText(o.status)}</span></div>
          <div class="kv-row"><span class="kv-key">联系方式</span><span class="kv-val">${escapeHtml(o.contact || '—')}</span></div>
          <div class="kv-row"><span class="kv-key">来源</span><span class="kv-val">${escapeHtml(o.source || '—')}</span></div>
          <div class="kv-row"><span class="kv-key">下单时间</span><span class="kv-val">${escapeHtml(o.created_at || '—')}</span></div>
        </div>
        ${secrets.length ? `
          <h4 style="margin-top:16px;font-size:13px">卡密</h4>
          ${secrets.map((s, i) => `
            <div class="secret-item">
              <code data-idx="${i}">${escapeHtml(s)}</code>
              <button class="btn btn-sm copy-btn" data-secret="${escapeHtml(s)}">复制</button>
            </div>
          `).join('')}
        ` : '<div style="margin-top:12px;color:var(--text-3);font-size:12px">暂无卡密</div>'}
        <div style="margin-top:12px"><a class="btn btn-secondary" href="#/order/${encodeURIComponent(o.trade_no)}">查看完整订单 →</a></div>
      </div>
    `;
    // 绑定复制按钮
    out.querySelectorAll('.copy-btn').forEach(b => {
      b.addEventListener('click', () => copySecret(b.dataset.secret));
    });
  } catch (e) {
    out.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

// ---------- 可下单商品列表（订单页）----------
async function loadOrderable() {
  const grid = $('#orderable-grid');
  if (!grid) return;
  grid.innerHTML = '<div class="loading">加载中…</div>';
  try {
    const r = await apiGet('/api/orders/items/list');
    if (r.code !== 200) {
      grid.innerHTML = `<div class="error">${escapeHtml(r.msg || '加载失败')}</div>`;
      return;
    }
    State.orderableCache = r.data || [];
    const items = State.orderableCache;
    if (!items.length) {
      grid.innerHTML = '<div class="empty" style="color:var(--text-3);font-size:13px;padding:20px;text-align:center">暂无可下单商品</div>';
      return;
    }
    grid.innerHTML = `
      <div class="orderable-grid">
        ${items.map(it => `
          <div class="orderable-card">
            <div class="orderable-cover"><img src="${escapeHtml(it.cover || '')}" onerror="this.style.display='none'" alt="" /></div>
            <div class="orderable-body">
              <div class="orderable-name">${escapeHtml(it.name)}</div>
              <div class="orderable-price"><span class="goods-price"><span class="goods-price-prefix">¥</span>${(it.price || 0).toFixed(2)}</span></div>
              <div class="orderable-code">code: ${escapeHtml(it.code)}</div>
              <button class="btn btn-primary orderable-btn" data-code="${escapeHtml(it.code)}">立即下单</button>
            </div>
          </div>
        `).join('')}
      </div>
    `;
    grid.querySelectorAll('.orderable-btn').forEach(b => {
      b.addEventListener('click', async () => {
        const code = b.dataset.code;
        // 通过 shared_code 下单时，从 /api/orders/items/list 数据组 drawer
        const it = items.find(x => x.code === code);
        if (!it) return;
        // 构造 detail 兼容 openOrderableDrawer
        const detail = {
          commodity: {
            id: 0,
            name: it.name,
            cover: it.cover || '',
            price: it.price,
            stock: typeof it.stock === 'number' ? it.stock : 99,
            delivery_label: it.delivery_way === 0 ? '手动发货' : '自动发货',
            source: 'upstream:upstreama',
            shared_code: it.code,
          },
          description: it.description || '',
          config: it.config || '',
          minimum: 1,
          maximum: typeof it.stock === 'number' ? it.stock : 99,
          password_status: 0,
          delivery_way: it.delivery_way || 1,
        };
        openOrderableDrawer(detail);
      });
    });
  } catch (e) {
    grid.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

// ---------- 工具 ----------
async function copySecret(text) {
  if (!text) return;
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      // 旧浏览器 fallback
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed'; ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
    }
    toast('已复制到剪贴板', 'ok');
  } catch (e) {
    toast('复制失败：' + e.message, 'err');
  }
}

function gotoOrder(tradeNo) {
  if (!tradeNo) return;
  location.hash = '#/order/' + encodeURIComponent(tradeNo);
}

function reopenDetail(id) {
  if (!id) return;
  location.hash = '#/commodity/' + id;
}

// 修复 #lookup-btn 在 renderOrders 里的 onclick → 改成 addEventListener
// 原来 HTML: <button class="btn btn-primary" onclick="Mastore.lookupOrder()">查询</button>
// 我们在 DOMContentLoaded 里已经绑定了 #lookup-btn → lookupOrder
// 这里提供 lookupOrder 函数本身（前面已定义）

})();
