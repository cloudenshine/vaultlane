// AdminApp 后台前端（独立命名空间，与前台 Vaultlane SPA 隔离）
const AdminApp = {
  state: { tab: 'dashboard', user: null },

  async init() {
    // 检查登录态（如果未登录 401，后端会跳 login 页）
    try {
      const r = await fetch('/admin/api/dashboard', { credentials: 'same-origin' });
      if (r.status === 401) { location.href = '/admin'; return; }
      const j = await r.json();
      this.state.user = j.user || 'admin';
      document.getElementById('user-name').textContent = this.state.user;
    } catch (e) {
      console.error(e);
    }

    // 路由
    window.addEventListener('hashchange', () => this.render());
    this.render();

    // 退出
    document.getElementById('logout').onclick = async () => {
      await fetch('/admin/api/logout', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'X-CSRF-Token': this.csrfToken() },
      });
      location.href = '/admin';
    };
  },

  csrfToken() {
    const m = document.cookie.match(/(?:^|; )admin_csrf=([^;]*)/);
    return m ? decodeURIComponent(m[1]) : '';
  },

  totpHeaders(extra) {
    const h = Object.assign({ 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrfToken() }, extra || {});
    const totp = this.state.totpCode || sessionStorage.getItem('admin_totp') || '';
    if (totp) h['X-TOTP-Code'] = totp;
    return h;
  },

  toast(msg, type) {
    let t = document.querySelector('.toast');
    if (!t) {
      t = document.createElement('div');
      t.className = 'toast';
      document.body.appendChild(t);
    }
    t.className = 'toast ' + (type || '');
    t.textContent = msg;
    t.style.display = 'block';
    setTimeout(() => t.style.display = 'none', 3000);
  },

  api(path, opts) {
    opts = opts || {};
    const headers = Object.assign({}, this.totpHeaders(), opts.headers || {});
    if ((opts.method || 'GET') === 'GET') delete headers['Content-Type'];
    return fetch('/admin/api' + path, Object.assign({ credentials: 'same-origin' }, opts, { headers }))
      .then(r => r.json());
  },

  postJSON(path, body, method) {
    return this.api(path, {
      method: method || 'POST',
      headers: this.totpHeaders(),
      body: JSON.stringify(body || {}),
    });
  },

  putJSON(path, body) {
    return this.api(path, {
      method: 'PUT',
      headers: this.totpHeaders(),
      body: JSON.stringify(body),
    });
  },

  render() {
    const hash = location.hash.replace('#', '') || 'dashboard';
    // 模块开关 → 隐藏标签页（避免跳到 404）
    const M = window.MODULES || {};
    const disabled = {
      'stats': M.finance_stats === false,
      'downstream': M.integrations === false,
      'settings': M.config_center === false,
      'coupons': M.coupon === false,
    };
    if (disabled[hash]) { location.hash = '#dashboard'; return; }
    this.state.tab = hash;
    document.querySelectorAll('.nav a').forEach(a => {
      const t = a.dataset.tab;
      if (disabled[t]) a.style.display = 'none';
      else a.classList.toggle('active', t === hash);
    });
    const fn = this['render_' + hash] || this.render_dashboard;
    fn.call(this);
  },

  // ---- Dashboard ----
  render_dashboard() {
    document.getElementById('main').innerHTML = '<div class="loading">加载中…</div>';
    Promise.all([
      this.api('/dashboard'),
      this.api('/stats/overview'),
    ]).then(([dashJ, statsJ]) => {
      const d = dashJ.data || {};
      const s = statsJ.data || {};
      const fmt = (n) => '¥' + (n || 0).toFixed(2);
      document.getElementById('main').innerHTML = `
        <div class="grid-3">
          <div class="card"><div class="card-head"><h2>今日订单</h2></div><div style="font-size:32px;font-weight:600;color:var(--brand)">${s.today_orders||0}</div><div style="font-size:12px;color:var(--ink-2);margin-top:4px">收入 ${fmt(s.today_revenue)}</div></div>
          <div class="card"><div class="card-head"><h2>本月订单</h2></div><div style="font-size:32px;font-weight:600;color:var(--brand)">${s.month_orders||0}</div><div style="font-size:12px;color:var(--ink-2);margin-top:4px">收入 ${fmt(s.month_revenue)}</div></div>
          <div class="card"><div class="card-head"><h2>累计</h2></div><div style="font-size:32px;font-weight:600;color:var(--brand)">${s.total_orders||0}</div><div style="font-size:12px;color:var(--ink-2);margin-top:4px">商品 ${s.total_commodities||0}</div></div>
        </div>
        <div class="card">
          <div class="card-head">
            <h2>近 30 天收入曲线</h2>
            <a class="btn btn-sm" href="#/stats">查看完整统计 →</a>
          </div>
          <div id="dash-revenue-chart"><div class="loading">加载中…</div></div>
        </div>
        <div class="card">
          <div class="card-head"><h2>上游连接 (upstreama)</h2></div>
          <div class="kv">
            <div>
              <div class="kv-row"><span class="kv-key">网关</span><span class="kv-val">${d.upstream?.base_url||'—'}</span></div>
              <div class="kv-row"><span class="kv-key">AppID</span><span class="kv-val">${d.upstream?.app_id||'—'}</span></div>
              <div class="kv-row"><span class="kv-key">总调用</span><span class="kv-val">${d.upstream?.total_calls||0}</span></div>
              <div class="kv-row"><span class="kv-key">成功</span><span class="kv-val" style="color:var(--ok)">${d.upstream?.success_calls||0}</span></div>
            </div>
            <div>
              <div class="kv-row"><span class="kv-key">失败</span><span class="kv-val" style="color:var(--err)">${d.upstream?.failed_calls||0}</span></div>
              <div class="kv-row"><span class="kv-key">平均延迟</span><span class="kv-val">${d.upstream?.avg_latency_ms||0} ms</span></div>
              <div class="kv-row"><span class="kv-key">缓存命中率</span><span class="kv-val">${d.cache?.hit_rate||'—'}</span></div>
              <div class="kv-row"><span class="kv-key">最后错误</span><span class="kv-val" style="font-size:12px;color:var(--err)">${(d.upstream?.last_error||'—').slice(0,40)}</span></div>
            </div>
          </div>
        </div>
        <div class="card">
          <div class="card-head"><h2>下游网关</h2></div>
          <div class="kv">
            <div>
              <div class="kv-row"><span class="kv-key">启用</span><span class="kv-val">${d.downstream?'<span class="badge badge-ok">已启用</span>':'<span class="badge badge-info">未启用</span>'}</span></div>
              <div class="kv-row"><span class="kv-key">AppKey</span><span class="kv-val">${d.downstream?.app_key||'—'}</span></div>
              <div class="kv-row"><span class="kv-key">SellerID</span><span class="kv-val">${d.downstream?.seller_id||0}</span></div>
            </div>
            <div>
              <div class="kv-row"><span class="kv-key">网关</span><span class="kv-val" style="font-size:12px">${d.downstream?.gateway_url||'—'}</span></div>
              <div class="kv-row"><span class="kv-key">总调用</span><span class="kv-val">${d.downstream?.total_calls||0}</span></div>
              <div class="kv-row"><span class="kv-key">失败</span><span class="kv-val" style="color:var(--err)">${d.downstream?.failed_calls||0}</span></div>
            </div>
          </div>
        </div>
      `;
      // 拉收入曲线并绘制迷你柱状图
      this.api('/stats/revenue?days=30').then(rj => {
        const points = rj.data || [];
        this.renderMiniBarChart('dash-revenue-chart', points);
      });
    });
  },

  // 迷你柱状图（不引外部库，SVG 原生）
  renderMiniBarChart(elId, points) {
    const el = document.getElementById(elId);
    if (!el) return;
    if (!points || !points.length) {
      el.innerHTML = '<div class="loading">暂无数据</div>';
      return;
    }
    const W = 800, H = 180, P = 30;
    const data = points.map(p => p.revenue || 0);
    const maxV = Math.max(...data, 1);
    const bw = (W - P * 2) / data.length;
    const bars = data.map((v, i) => {
      const h = (v / maxV) * (H - P);
      const x = P + i * bw;
      const y = H - P - h;
      return `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${(bw*0.7).toFixed(1)}" height="${h.toFixed(1)}" fill="var(--brand)" opacity="0.85"><title>${points[i].date || ''} · ¥${v.toFixed(2)}</title></rect>`;
    }).join('');
    // 0 基线
    const baseLine = `<line x1="${P}" y1="${H-P}" x2="${W-P}" y2="${H-P}" stroke="var(--border)" />`;
    // Y 轴标签（最高点）
    const yLabel = `<text x="${P}" y="${P-5}" font-size="11" fill="var(--ink-2)">¥${maxV.toFixed(0)}</text>`;
    el.innerHTML = `<svg viewBox="0 0 ${W} ${H}" style="width:100%;height:auto;max-height:200px">${baseLine}${bars}${yLabel}</svg>`;
  },

  // ---- Stats / 财务统计 ----
  render_stats() {
    document.getElementById('main').innerHTML = '<div class="loading">加载中…</div>';
    Promise.all([
      this.api('/stats/overview'),
      this.api('/stats/revenue?days=30'),
      this.api('/stats/top_commodities?limit=10'),
    ]).then(([ovJ, revJ, topJ]) => {
      const s = ovJ.data || {};
      const rev = revJ.data || [];
      const top = topJ.data || [];
      const fmt = (n) => '¥' + (n || 0).toFixed(2);
      const totalRev = rev.reduce((sum, p) => sum + (p.revenue || 0), 0);
      document.getElementById('main').innerHTML = `
        <div class="grid-3">
          <div class="card"><div class="card-head"><h2>今日</h2></div><div style="font-size:24px;font-weight:600">${s.today_orders||0}</div><div style="font-size:12px;color:var(--ink-2);margin-top:4px">${fmt(s.today_revenue)}</div></div>
          <div class="card"><div class="card-head"><h2>本月</h2></div><div style="font-size:24px;font-weight:600">${s.month_orders||0}</div><div style="font-size:12px;color:var(--ink-2);margin-top:4px">${fmt(s.month_revenue)}</div></div>
          <div class="card"><div class="card-head"><h2>近 30 天总收入</h2></div><div style="font-size:24px;font-weight:600;color:var(--brand)">${fmt(totalRev)}</div><div style="font-size:12px;color:var(--ink-2);margin-top:4px">${rev.reduce((a,b)=>a+(b.orders||0),0)} 单</div></div>
        </div>
        <div class="card">
          <div class="card-head"><h2>30 天收入曲线</h2></div>
          <div id="stats-revenue-chart"><div class="loading">加载中…</div></div>
        </div>
        <div class="card">
          <div class="card-head"><h2>销量 Top 10</h2></div>
          ${top.length ? `
            <table>
              <thead><tr><th>#</th><th>商品</th><th>销量</th><th>收入</th></tr></thead>
              <tbody>${top.map((c, i) => `
                <tr>
                  <td>${i+1}</td>
                  <td>${this.escapeHtml(c.name||'#'+c.commodity_id)}</td>
                  <td>${c.sold||0}</td>
                  <td>${fmt(c.revenue)}</td>
                </tr>`).join('')}</tbody>
            </table>` : '<div class="loading">暂无销量数据</div>'}
        </div>
        <div class="card">
          <div class="card-head"><h2>支付记录</h2></div>
          <div class="form" style="display:flex;gap:8px;align-items:flex-end;margin-bottom:12px">
            <div class="field" style="flex:1;margin:0">
              <label>状态</label>
              <select id="pay-status">
                <option value="-1">全部</option>
                <option value="0">待支付</option>
                <option value="1">已支付</option>
                <option value="2">已退款</option>
                <option value="3">失败</option>
              </select>
            </div>
            <div class="field" style="flex:1;margin:0">
              <label>支付方式</label>
              <select id="pay-method">
                <option value="">全部</option>
                <option value="balance">余额</option>
                <option value="epay">EPay</option>
                <option value="usdt">USDT</option>
              </select>
            </div>
            <button class="btn btn-primary" onclick="AdminApp.loadPayments()">查询</button>
          </div>
          <div id="payments-table"><div class="loading">选择条件查询</div></div>
        </div>
      `;
      this.renderMiniBarChart('stats-revenue-chart', rev);
    });
  },

  loadPayments() {
    const st = document.getElementById('pay-status').value;
    const m = document.getElementById('pay-method').value;
    const qs = new URLSearchParams();
    if (st !== '-1') qs.set('status', st);
    if (m) qs.set('method', m);
    qs.set('limit', '50');
    document.getElementById('payments-table').innerHTML = '<div class="loading">查询中…</div>';
    this.api('/stats/payments?' + qs).then(j => {
      const list = j.data || [];
      document.getElementById('payments-table').innerHTML = list.length ? `
        <table>
          <thead><tr><th>ID</th><th>订单</th><th>方式</th><th>金额</th><th>状态</th><th>创建</th></tr></thead>
          <tbody>${list.map(p => `
            <tr>
              <td>${p.id}</td>
              <td><code style="font-size:11px">${p.trade_no||'—'}</code></td>
              <td>${this.escapeHtml(p.method||'—')}</td>
              <td>¥${(p.amount||0).toFixed(2)}</td>
              <td>${['待支付','已支付','已退款','失败'][p.status]||'—'}</td>
              <td>${(p.created_at||'').slice(0,16)}</td>
            </tr>`).join('')}</tbody>
        </table>` : '<div class="loading">无记录</div>';
    });
  },

  // ---- Commodities ----
  render_commodities() {
    document.getElementById('main').innerHTML = `
      <div class="card">
        <div class="card-head">
          <h2>商品管理</h2>
          <button class="btn btn-primary" onclick="AdminApp.newCommodity()">+ 新建商品</button>
        </div>
        <div id="commodity-list"><div class="loading">加载中…</div></div>
      </div>
    `;
    this.api('/commodities').then(j => {
      const list = j.data || [];
      if (!list.length) { document.getElementById('commodity-list').innerHTML = '<div class="loading">暂无商品</div>'; return; }
      const html = `
        <table>
          <thead><tr><th>ID</th><th>名称</th><th>来源</th><th>售价</th><th>库存</th><th>已售</th><th>状态</th><th>操作</th></tr></thead>
          <tbody>${list.map(c => `
            <tr>
              <td>${c.id}</td>
              <td>${this.escapeHtml(c.name)}</td>
              <td>${this.sourceBadge(c.source)}</td>
              <td>¥${(c.price||0).toFixed(2)}</td>
              <td>${c.stock}</td>
              <td>${c.sold}</td>
              <td>${c.status===1?'<span class="badge badge-ok">上架</span>':'<span class="badge badge-info">下架</span>'}</td>
              <td>
                <button class="btn btn-sm" onclick="AdminApp.editCommodity(${c.id})">编辑</button>
                ${c.source==='self'?`<button class="btn btn-sm" onclick="AdminApp.manageSecrets(${c.id},'${this.escapeHtml(c.name)}')">卡密</button>`:''}
                ${c.source==='self'?`<button class="btn btn-sm" onclick="AdminApp.syncToDownstream(${c.id})">同步下游网关</button>`:''}
                <button class="btn btn-sm btn-danger" onclick="AdminApp.deleteCommodity(${c.id})">删除</button>
              </td>
            </tr>`).join('')}</tbody>
        </table>`;
      document.getElementById('commodity-list').innerHTML = html;
    });
  },

  newCommodity() {
    const html = `
      <div class="card">
        <div class="card-head"><h2>新建商品</h2><button class="btn" onclick="AdminApp.render_commodities()">取消</button></div>
        <div class="form">
          <div class="field"><label>名称</label><input id="f-name" /></div>
          <div class="field"><label>封面 URL</label><input id="f-cover" placeholder="https://..." /></div>
          <div class="field"><label>售价 (元)</label><input id="f-price" type="number" step="0.01" value="0" /></div>
          <div class="field"><label>成本 (元)</label><input id="f-cost" type="number" step="0.01" value="0" /></div>
          <div class="field"><label>分类 ID</label><input id="f-cat" type="number" value="0" /></div>
          <div class="field"><label>发货方式</label>
            <select id="f-delivery"><option value="1">自动卡密</option><option value="0">手动发货</option></select>
          </div>
          <div class="field"><label>商品描述</label><textarea id="f-desc"></textarea></div>
          <button class="btn btn-primary" onclick="AdminApp.saveCommodity()">保存</button>
        </div>
      </div>`;
    document.getElementById('main').innerHTML = html;
  },

  editCommodity(id) {
    this.api('/commodities').then(j => {
      const c = (j.data || []).find(x => x.id === id);
      if (!c) return;
      const html = `
        <div class="card">
          <div class="card-head"><h2>编辑商品 #${id}</h2><button class="btn" onclick="AdminApp.render_commodities()">取消</button></div>
          <div class="form">
            <div class="field"><label>名称</label><input id="f-name" value="${this.escapeHtml(c.name)}" /></div>
            <div class="field"><label>封面 URL</label><input id="f-cover" value="${this.escapeHtml(c.cover)}" /></div>
            <div class="field"><label>售价</label><input id="f-price" type="number" step="0.01" value="${c.price}" /></div>
            <div class="field"><label>成本</label><input id="f-cost" type="number" step="0.01" value="${c.cost_price}" /></div>
            <div class="field"><label>分类 ID</label><input id="f-cat" type="number" value="${c.category_id}" /></div>
            <div class="field"><label>状态</label>
              <select id="f-status"><option value="1" ${c.status===1?'selected':''}>上架</option><option value="0" ${c.status===0?'selected':''}>下架</option></select>
            </div>
            <div class="field"><label>描述</label><textarea id="f-desc">${this.escapeHtml(c.description||'')}</textarea></div>
            <button class="btn btn-primary" onclick="AdminApp.saveCommodity(${id})">保存</button>
          </div>
        </div>`;
      document.getElementById('main').innerHTML = html;
    });
  },

  saveCommodity(id) {
    const body = {
      name: val('f-name'),
      cover: val('f-cover'),
      price: parseFloat(val('f-price')) || 0,
      cost_price: parseFloat(val('f-cost')) || 0,
      category_id: parseInt(val('f-cat')) || 0,
      delivery_way: parseInt(val('f-delivery') || '1'),
      status: parseInt(val('f-status') || '1'),
      description: val('f-desc'),
      source: 'self',
    };
    const p = id ? this.putJSON('/commodities/' + id, body) : this.postJSON('/commodities', body);
    p.then(j => {
      if (j.code === 200) { this.toast('已保存', 'ok'); this.render_commodities(); }
      else this.toast(j.msg || '保存失败', 'err');
    });
  },

  deleteCommodity(id) {
    if (!confirm('删除商品将同时删除所有卡密，确定？')) return;
    this.api('/commodities/' + id, { method: 'DELETE' }).then(j => {
      if (j.code === 200) { this.toast('已删除', 'ok'); this.render_commodities(); }
      else this.toast(j.msg, 'err');
    });
  },

  manageSecrets(id, name) {
    const html = `
      <div class="card">
        <div class="card-head">
          <h2>卡密管理 - ${this.escapeHtml(name)}</h2>
          <div>
            <button class="btn" onclick="AdminApp.render_commodities()">返回</button>
            <button class="btn btn-primary" onclick="AdminApp.showImport(${id})">+ 批量导入</button>
          </div>
        </div>
        <div id="secret-list"><div class="loading">加载中…</div></div>
      </div>`;
    document.getElementById('main').innerHTML = html;
    this.loadSecrets(id);
  },

  loadSecrets(commID) {
    this.api('/secrets?commodity_id=' + commID + '&limit=100').then(j => {
      const list = j.data || [];
      document.getElementById('secret-list').innerHTML = `
        <table>
          <thead><tr><th>ID</th><th>内容</th><th>状态</th><th>订单 ID</th><th>售出时间</th></tr></thead>
          <tbody>${list.length ? list.map(s => `
            <tr>
              <td>${s.id}</td>
              <td><code>${this.escapeHtml(s.content).slice(0,80)}</code></td>
              <td>${statusBadge_outer(s.status)}</td>
              <td>${s.order_id||'—'}</td>
              <td>${s.sold_at||'—'}</td>
            </tr>`).join('') : '<tr><td colspan="5" class="loading">暂无卡密</td></tr>'}</tbody>
        </table>`;
    });
  },

  showImport(commID) {
    const html = `
      <div class="card">
        <div class="card-head"><h2>批量导入卡密</h2></div>
        <div class="form">
          <div class="field"><label>每行一个卡密</label><textarea id="f-secrets" rows="10" placeholder="CARD-A1-0001&#10;CARD-A1-0002&#10;CARD-A1-0003"></textarea></div>
          <button class="btn btn-primary" onclick="AdminApp.importSecrets(${commID})">导入</button>
        </div>
      </div>`;
    document.getElementById('main').innerHTML = html;
  },

  importSecrets(commID) {
    const contents = val('f-secrets').split(/\r?\n/).map(s => s.trim()).filter(Boolean);
    if (!contents.length) { this.toast('请输入卡密', 'err'); return; }
    this.postJSON('/secrets/import', { commodity_id: commID, contents }).then(j => {
      if (j.code === 200) { this.toast('导入 ' + j.data.imported + ' 条', 'ok'); this.loadSecrets(commID); }
      else this.toast(j.msg, 'err');
    });
  },

  syncToDownstream(id) {
    if (!confirm('将此商品同步到下游网关？')) return;
    this.postJSON('/downstream/products/sync', { commodity_id: id }).then(j => {
      if (j.code === 200) this.toast('已同步，ProductID=' + j.data.product_id, 'ok');
      else this.toast(j.msg, 'err');
    });
  },

  // ---- Orders ----
  render_orders() {
    document.getElementById('main').innerHTML = '<div class="card"><div class="loading">加载中…</div></div>';
    this.api('/orders').then(j => {
      const list = j.data || [];
      const html = `
        <div class="card">
          <div class="card-head"><h2>最近订单（${list.length}）</h2></div>
          ${list.length ? `
            <table>
              <thead><tr><th>订单号</th><th>来源</th><th>商品</th><th>联系方式</th><th>数量</th><th>金额</th><th>状态</th><th>创建时间</th></tr></thead>
              <tbody>${list.map(o => `
                <tr>
                  <td><code style="font-size:11px">${o.trade_no}</code></td>
                  <td>${this.sourceBadge(o.source)}</td>
                  <td>${this.escapeHtml(o.commodity_name||'#'+o.commodity_id)}</td>
                  <td>${this.escapeHtml(o.contact||'—')}</td>
                  <td>${o.num}</td>
                  <td>¥${(o.amount||0).toFixed(2)}</td>
                  <td>${orderStatusBadge_outer(o.status)}</td>
                  <td>${(o.created_at||'').slice(0,16)}</td>
                </tr>`).join('')}</tbody>
            </table>` : '<div class="loading">暂无订单</div>'}
        </div>`;
      document.getElementById('main').innerHTML = html;
    });
  },

  // ---- Secrets ----
  render_secrets() {
    document.getElementById('main').innerHTML = `
      <div class="card">
        <div class="card-head"><h2>卡密查询</h2></div>
        <div class="form">
          <div class="field"><label>商品 ID</label><input id="f-comm" type="number" placeholder="留空查全部" /></div>
          <div class="field"><label>状态</label>
            <select id="f-status"><option value="">全部</option><option value="0">未售</option><option value="1">已售</option><option value="2">锁定</option></select>
          </div>
          <button class="btn btn-primary" onclick="AdminApp.searchSecrets()">查询</button>
        </div>
        <div id="result" style="margin-top:16px"></div>
      </div>`;
  },

  searchSecrets() {
    const comm = val('f-comm');
    const st = val('f-status');
    const qs = new URLSearchParams();
    if (comm) qs.set('commodity_id', comm);
    if (st !== '') qs.set('status', st);
    qs.set('limit', '100');
    this.api('/secrets?' + qs).then(j => {
      const list = j.data || [];
      document.getElementById('result').innerHTML = list.length ? `
        <table>
          <thead><tr><th>ID</th><th>商品</th><th>内容</th><th>状态</th><th>订单</th></tr></thead>
          <tbody>${list.map(s => `
            <tr>
              <td>${s.id}</td><td>#${s.commodity_id}</td>
              <td><code>${this.escapeHtml(s.content).slice(0,60)}</code></td>
              <td>${statusBadge_outer(s.status)}</td>
              <td>${s.order_id||'—'}</td>
            </tr>`).join('')}</tbody>
        </table>` : '<div class="loading">无结果</div>';
    });
  },

  // ---- Downstream ----
  render_downstream() {
    document.getElementById('main').innerHTML = `
      <div class="card">
        <div class="card-head">
          <h2>下游网关</h2>
          <div>
            <button class="btn" onclick="AdminApp.downstreamTest()">连通测试</button>
            <button class="btn btn-primary" onclick="AdminApp.downstreamListProducts()">拉商品列表</button>
          </div>
        </div>
        <div id="downstream-content"><div class="loading">点击按钮测试</div></div>
      </div>`;
  },

  downstreamTest() {
    document.getElementById('downstream-content').innerHTML = '<div class="loading">测试中…</div>';
    this.api('/downstream/test').then(j => {
      const ok = j.ok;
      document.getElementById('downstream-content').innerHTML = `
        <div class="card" style="margin:0">
          <div style="font-size:18px;color:${ok?'var(--ok)':'var(--err)'}">${ok ? '✓ 连通成功' : '✗ 连通失败'}</div>
          ${j.err ? `<div style="color:var(--err);margin-top:8px;font-size:13px">${this.escapeHtml(j.err)}</div>` : ''}
          ${j.metrics ? `<div class="kv" style="margin-top:16px">
            <div class="kv-row"><span class="kv-key">AppKey</span><span class="kv-val">${j.metrics.app_key}</span></div>
            <div class="kv-row"><span class="kv-key">SellerID</span><span class="kv-val">${j.metrics.seller_id}</span></div>
            <div class="kv-row"><span class="kv-key">网关</span><span class="kv-val" style="font-size:11px">${j.metrics.gateway_url}</span></div>
            <div class="kv-row"><span class="kv-key">总调用</span><span class="kv-val">${j.metrics.total_calls}</span></div>
            <div class="kv-row"><span class="kv-key">成功</span><span class="kv-val">${j.metrics.success_calls}</span></div>
            <div class="kv-row"><span class="kv-key">失败</span><span class="kv-val" style="color:var(--err)">${j.metrics.failed_calls}</span></div>
          </div>` : ''}
        </div>`;
    });
  },

  downstreamListProducts() {
    document.getElementById('downstream-content').innerHTML = '<div class="loading">加载中…</div>';
    this.api('/downstream/products?page=1&size=20').then(j => {
      const list = j.data?.list || [];
      document.getElementById('downstream-content').innerHTML = list.length ? `
        <table>
          <thead><tr><th>ProductID</th><th>标题</th><th>售价(分)</th><th>库存</th><th>已售</th><th>状态</th></tr></thead>
          <tbody>${list.map(p => `
            <tr>
              <td>${p.product_id}</td>
              <td>${this.escapeHtml(p.title||'—')}</td>
              <td>¥${(p.price/100).toFixed(2)}</td>
              <td>${p.stock}</td>
              <td>${p.sold}</td>
              <td>${p.product_status}</td>
            </tr>`).join('')}</tbody>
        </table>` : '<div class="loading">暂无商品或接口失败</div>';
    });
  },

  // ---- Settings ----
  render_settings() {
    document.getElementById('main').innerHTML = '<div class="card"><div class="loading">加载中…</div></div>';
    this.api('/settings').then(j => {
      const d = j.data || {};
      const u = d.upstream || {};
      const x = d.downstreamb || {};
      const m = d.mock || {};
      const mods = d.modules || {};
      const modChips = [
        ['coupon', '优惠券'],
        ['referral', '邀请返佣'],
        ['email', '邮件通知'],
        ['upstream_sync', '上游同步'],
        ['downstream_callback', '下游网关回调'],
        ['finance_stats', '财务统计'],
        ['config_center', '配置中心'],
        ['integrations', '对接面板'],
      ].map(([k, label]) => {
        const on = mods[k] === true;
        return `<span class="badge ${on?'badge-ok':'badge-info'}" style="margin:2px">${label}：${on?'开':'关'}</span>`;
      }).join(' ');
      document.getElementById('main').innerHTML = `
        <div class="card">
          <div class="card-head"><h2>模块开关（运行时状态）</h2></div>
          <p style="color:var(--ink-2);font-size:12px;margin-bottom:10px">模块开关在 <code>config.yaml</code> 的 <code>modules:</code> 段配置，重启后生效。</p>
          <div>${modChips}</div>
        </div>
        <div class="card">
          <div class="card-head"><h2>上游网关 (upstreama)</h2></div>
          <div class="form">
            <div class="field"><label>网关地址</label><input value="${this.escapeHtml(u.base_url||'')}" disabled /></div>
            <div class="field"><label>AppID</label><input value="${this.escapeHtml(u.app_id||'')}" disabled /></div>
            <p style="color:var(--ink-2);font-size:12px">上游参数由 config.yaml 配置，重启生效</p>
          </div>
        </div>
        <div class="card">
          <div class="card-head"><h2>下游网关</h2></div>
          <div class="kv">
            <div>
              <div class="kv-row"><span class="kv-key">启用</span><span class="kv-val">${x.enabled?'<span class="badge badge-ok">已启用</span>':'<span class="badge badge-info">未启用</span>'}</span></div>
              <div class="kv-row"><span class="kv-key">AppKey</span><span class="kv-val">${x.app_key||'—'}</span></div>
            </div>
            <div>
              <div class="kv-row"><span class="kv-key">SellerID</span><span class="kv-val">${x.seller_id||0}</span></div>
              <div class="kv-row"><span class="kv-key">网关</span><span class="kv-val" style="font-size:11px">${x.base_url||'—'}</span></div>
            </div>
          </div>
        </div>
        <div class="card">
          <div class="card-head"><h2>限流</h2></div>
          <div class="form">
            <div class="field">
              <label>每 IP 每分钟请求数</label>
              <input id="set-ratelimit" type="number" min="1" value="${d.rate_limit||60}" />
            </div>
            <button class="btn btn-primary" onclick="AdminApp.saveRateLimit()">保存</button>
          </div>
        </div>
        <div class="card">
          <div class="card-head"><h2>Mock 模式</h2></div>
          <div class="form">
            <div class="field"><label>当前状态</label>
              <span style="color:${m.enabled?'var(--warn)':'var(--ink-2)'};font-size:14px">${m.enabled ? '✓ 已开启' : '✗ 未启用'}</span>
            </div>
            <div class="field"><label>卡密前缀</label>
              <input id="set-mock-prefix" value="${this.escapeHtml(m.prefix||'MOCK-')}" />
            </div>
            <div class="field"><label>延迟 (ms)</label>
              <input id="set-mock-delay" type="number" min="0" value="${m.delay_ms||0}" />
            </div>
            <p style="color:var(--ink-2);font-size:12px;margin-bottom:8px">开启后上游接口返回模拟卡密，无需真实余额即可演示全链路。</p>
            <div style="display:flex;gap:8px">
              <button class="btn ${m.enabled?'btn-danger':'btn-primary'}" onclick="AdminApp.toggleMock(${!m.enabled})">
                ${m.enabled ? '关闭 Mock' : '开启 Mock'}
              </button>
              <button class="btn" onclick="AdminApp.saveMockConfig()">保存前缀/延迟</button>
            </div>
          </div>
        </div>
        <div class="card">
          <div class="card-head"><h2>TOTP 双因子认证</h2></div>
          <div class="form">
            <p style="color:var(--ink-2);font-size:12px;margin-bottom:8px">高危操作（改价/删除/关闭 TOTP）启用后需携带 6 位验证码。</p>
            <div class="field"><label>当前会话验证码（可选缓存）</label>
              <input id="set-totp-code" placeholder="6 位数字" maxlength="6" />
            </div>
            <div style="display:flex;gap:8px;flex-wrap:wrap">
              <button class="btn" onclick="AdminApp.saveTotpSession()">缓存验证码</button>
              <button class="btn btn-primary" onclick="AdminApp.setupTotp()">生成密钥</button>
              <button class="btn" onclick="AdminApp.confirmTotp()">确认激活</button>
              <button class="btn btn-danger" onclick="AdminApp.disableTotp()">禁用</button>
            </div>
            <pre id="totp-setup-box" style="white-space:pre-wrap;font-size:12px;margin-top:10px"></pre>
          </div>
        </div>
      `;
    });
  },

  toggleMock(enabled) {
    // VULN-015 修复：需要输入 admin 密码二次确认
    const confirmPwd = prompt('切换 mock 模式需要输入 admin 密码：');
    if (!confirmPwd) { this.toast('已取消', 'err'); return; }
    this.postJSON('/settings/mock', { enabled, confirm: confirmPwd }).then(j => {
      if (j.code === 200) { this.toast('已' + (enabled?'开启':'关闭'), 'ok'); this.render_settings(); }
      else this.toast(j.msg, 'err');
    });
  },

  saveMockConfig() {
    const body = {
      mock: {
        enabled: true, // 不变（保存时复用当前状态也可）
        prefix: document.getElementById('set-mock-prefix').value,
        delay_ms: parseInt(document.getElementById('set-mock-delay').value) || 0,
      },
    };
    this.putJSON('/settings', body).then(j => {
      if (j.code === 200) { this.toast('已保存', 'ok'); this.render_settings(); }
      else this.toast(j.msg, 'err');
    });
  },

  saveTotpSession() {
    const code = (document.getElementById('set-totp-code')?.value || '').trim();
    this.state.totpCode = code;
    if (code) sessionStorage.setItem('admin_totp', code);
    else sessionStorage.removeItem('admin_totp');
    this.toast(code ? '已缓存 TOTP' : '已清除 TOTP', 'ok');
  },
  async setupTotp() {
    const j = await this.postJSON('/totp/setup', {});
    const box = document.getElementById('totp-setup-box');
    if (j.code === 200) {
      const d = j.data || {};
      box.textContent = '密钥: ' + (d.secret || '') + '\nURI: ' + (d.otp_url || '') + '\n请用 Authenticator 扫描后点「确认激活」';
    } else this.toast(j.msg || '生成失败', 'err');
  },
  async confirmTotp() {
    const code = prompt('请输入 Authenticator 当前 6 位验证码：');
    if (!code) return;
    const j = await this.postJSON('/totp/confirm', { code });
    if (j.code === 200) { this.toast('TOTP 已激活', 'ok'); this.render_settings(); }
    else this.toast(j.msg || '激活失败', 'err');
  },
  async disableTotp() {
    const code = prompt('禁用 TOTP 需要当前验证码：');
    if (!code) return;
    this.state.totpCode = code;
    const j = await this.postJSON('/totp/disable', { code });
    if (j.code === 200) { this.toast('已禁用', 'ok'); this.render_settings(); }
    else this.toast(j.msg || '禁用失败', 'err');
  },

  saveRateLimit() {
    const body = { rate_limit: parseInt(document.getElementById('set-ratelimit').value) || 60 };
    this.putJSON('/settings', body).then(j => {
      if (j.code === 200) { this.toast('已保存（重启后生效）', 'ok'); this.render_settings(); }
      else this.toast(j.msg, 'err');
    });
  },

  render_coupons() {
    document.getElementById('main').innerHTML = '<div class="loading">加载中…</div>';
    this.api('/coupons').then(j => {
      const rows = j.data || [];
      document.getElementById('main').innerHTML = `
        <div class="card">
          <div class="card-head"><h2>优惠券</h2></div>
          <div class="form" style="display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:8px;margin-bottom:16px">
            <input id="cp-code" placeholder="券码 NEW10" />
            <input id="cp-name" placeholder="名称" />
            <select id="cp-type"><option value="0">固定立减</option><option value="1">百分比</option></select>
            <input id="cp-discount" type="number" step="0.01" placeholder="减免/折扣" />
            <input id="cp-min" type="number" step="0.01" placeholder="门槛金额" />
            <input id="cp-limit" type="number" placeholder="发放总量 0=不限" />
            <button class="btn btn-primary" onclick="AdminApp.createCoupon()">创建</button>
          </div>
          <table>
            <thead><tr><th>券码</th><th>名称</th><th>类型</th><th>减免</th><th>门槛</th><th>已用/总量</th><th>状态</th></tr></thead>
            <tbody>${rows.length ? rows.map(c => `<tr>
              <td><code>${this.escapeHtml(c.code||'')}</code></td>
              <td>${this.escapeHtml(c.name||'')}</td>
              <td>${c.type===1?'折扣':'立减'}</td>
              <td>${c.type===1?((c.discount||0)*100).toFixed(0)+'%':'¥'+(c.discount||0).toFixed(2)}</td>
              <td>¥${(c.min_amount||0).toFixed(2)}</td>
              <td>${c.used_count||0}/${c.total_limit||'∞'}</td>
              <td>${c.status===1?'启用':'停用'}</td>
            </tr>`).join('') : '<tr><td colspan="7">暂无优惠券</td></tr>'}</tbody>
          </table>
        </div>`;
    });
  },

  async createCoupon() {
    const body = {
      code: document.getElementById('cp-code').value.trim(),
      name: document.getElementById('cp-name').value.trim(),
      type: parseInt(document.getElementById('cp-type').value, 10) || 0,
      discount: parseFloat(document.getElementById('cp-discount').value) || 0,
      min_amount: parseFloat(document.getElementById('cp-min').value) || 0,
      total_limit: parseInt(document.getElementById('cp-limit').value, 10) || 0,
      status: 1,
    };
    const j = await this.postJSON('/coupons', body);
    if (j.code === 200) { this.toast('已创建', 'ok'); this.render_coupons(); }
    else this.toast(j.msg || '创建失败', 'err');
  },

// ---- 工具函数（AdminApp 方法，供模板字符串调用）----
val(id) { return document.getElementById(id).value; },
escapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/[&<>"']/g, c => ({
    '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'
  }[c]));
},
sourceBadge(s) {
  if (s === 'self') return '<span class="badge badge-ok">自营</span>';
  if (s && s.startsWith('upstream:')) return `<span class="badge badge-info">${this.escapeHtml(s)}</span>`;
  return `<span class="badge badge-warn">${this.escapeHtml(s||'')}</span>`;
},
statusBadge(st) { return ['未售','已售','锁定','禁用'][st] || '—'; },
orderStatusBadge(st) { return ['待支付','已支付','已发货','已退款'][st] || '—'; },

// ==================== 商品池 (v4) ====================
render_pool() {  document.getElementById('main').innerHTML = `
    <div class="card">
      <div class="card-head">
        <h2>商品池</h2>
        <div>
          <select id="pool-source" onchange="AdminApp.render_pool_list()">
            <option value="">全部来源</option>
          </select>
          <input id="pool-keyword" placeholder="搜索商品名" onkeyup="if(event.key==='Enter')AdminApp.render_pool_list()" />
          <button class="btn" onclick="AdminApp.render_pool_list()">搜索</button>
          <button class="btn btn-primary" onclick="AdminApp.syncUpstream()">⬇ 同步上游</button>
        </div>
      </div>

      <div class="pool-toolbar">
        <span>已选 <b id="pool-selected">0</b> 项</span>
        <button class="btn" onclick="AdminApp.poolBatchStatus(1)">批量上架</button>
        <button class="btn" onclick="AdminApp.poolBatchStatus(0)">批量下架</button>
        <span style="margin-left:12px">改价：</span>
        <select id="pool-price-type">
          <option value="0">固定金额</option>
          <option value="1">百分比</option>
        </select>
        <select id="pool-price-op"><option value="+">+</option><option value="-">-</option></select>
        <input id="pool-price-amt" type="number" step="0.01" min="0" placeholder="数量" style="width:80px" />
        <button class="btn" onclick="AdminApp.poolBatchPrice()">应用</button>
        <button class="btn btn-danger" onclick="AdminApp.poolBatchDelete()">批量删除</button>
      </div>

      <div id="pool-stats" class="pool-stats"></div>
      <div id="pool-list"><div class="loading">加载中…</div></div>
    </div>
  `;
  this.loadUpstreams();
  this.render_pool_list();
},

async loadUpstreams() {
  const j = await this.api('/pool/upstreams');
  const sel = document.getElementById('pool-source');
  if (!sel) return;
  (j.data || []).forEach(u => {
    const opt = document.createElement('option');
    opt.value = 'upstream:' + u.name;
    opt.textContent = `${u.name} (${u.type})`;
    sel.appendChild(opt);
  });
},

async render_pool_list() {
  const source = document.getElementById('pool-source')?.value || '';
  const keyword = document.getElementById('pool-keyword')?.value || '';
  const params = new URLSearchParams({ source, keyword, page: '1', limit: '100' });
  const j = await this.api('/pool?' + params);
  const list = j.data || [];
  const stats = j.stats || {};
  // 统计
  const stEl = document.getElementById('pool-stats');
  if (stEl) {
    const by = stats.by_source || {};
    stEl.innerHTML = Object.keys(by).length
      ? Object.entries(by).map(([k, v]) => `<span class="chip">${k}: <b>${v}</b>件</span>`).join(' ')
      : '<span class="chip">商品池为空，请先同步上游</span>';
  }
  if (!list.length) {
    document.getElementById('pool-list').innerHTML = '<div class="loading">商品池为空。点击"⬇ 同步上游"把上游商品拉进来。</div>';
    return;
  }
  const html = `
    <table class="pool-table">
      <thead><tr>
        <th><input type="checkbox" id="pool-check-all" onchange="AdminApp.poolToggleAll(this.checked)" /></th>
        <th>ID</th><th>名称</th><th>来源</th><th>分类</th>
        <th>成本</th><th>售价</th><th>库存</th><th>已售</th><th>状态</th><th>操作</th>
      </tr></thead>
      <tbody>
        ${list.map(c => `
          <tr>
            <td><input type="checkbox" class="pool-check" data-id="${c.id}" /></td>
            <td>${c.id}</td>
            <td><a href="javascript:" onclick="AdminApp.editPool(${c.id})">${this.escapeHtml(c.name||'').slice(0,40)}</a></td>
            <td>${this.sourceBadge(c.source)}</td>
            <td>${c.category_id || '—'}</td>
            <td>¥${(c.cost_price||0).toFixed(2)}</td>
            <td><b>¥${(c.sale_price||c.price||0).toFixed(2)}</b></td>
            <td>${c.stock}</td>
            <td>${c.sold}</td>
            <td>${c.status===1?'<span class="badge badge-ok">上架</span>':'<span class="badge badge-info">下架</span>'}</td>
            <td>
              <button class="btn btn-sm" onclick="AdminApp.poolToggle(${c.id}, ${c.status===1?0:1})">${c.status===1?'下架':'上架'}</button>
              <button class="btn btn-sm" onclick="AdminApp.editPool(${c.id})">编辑</button>
              <button class="btn btn-sm btn-danger" onclick="AdminApp.poolDelete(${c.id})">删</button>
            </td>
          </tr>`).join('')}
      </tbody>
    </table>
  `;
  document.getElementById('pool-list').innerHTML = html;
  this.updatePoolSelected();
},

poolToggleAll(checked) {
  document.querySelectorAll('.pool-check').forEach(cb => cb.checked = checked);
  this.updatePoolSelected();
},

updatePoolSelected() {
  const n = document.querySelectorAll('.pool-check:checked').length;
  const el = document.getElementById('pool-selected');
  if (el) el.textContent = n;
  document.querySelectorAll('.pool-check').forEach(cb => {
    cb.addEventListener('change', () => this.updatePoolSelected());
  });
},

poolSelectedIds() {
  return Array.from(document.querySelectorAll('.pool-check:checked')).map(cb => parseInt(cb.dataset.id, 10));
},

async poolBatchStatus(status) {
  const ids = this.poolSelectedIds();
  if (!ids.length) return this.toast('请先勾选商品', 'err');
  const j = await this.postJSON('/pool/status', { ids, status });
  if (j.code === 200) { this.toast('已' + (status?'上架':'下架') + ' ' + j.data.affected + ' 件', 'ok'); this.render_pool_list(); }
},

async poolBatchPrice() {
  const ids = this.poolSelectedIds();
  if (!ids.length) return this.toast('请先勾选商品', 'err');
  const type = parseInt(document.getElementById('pool-price-type').value, 10);
  const op = document.getElementById('pool-price-op').value;
  const amount = parseFloat(document.getElementById('pool-price-amt').value);
  if (!amount || amount <= 0) return this.toast('请填金额', 'err');
  const j = await this.postJSON('/pool/price', { ids, type, op, amount });
  if (j.code === 200) { this.toast('已调价 ' + j.data.affected + ' 件', 'ok'); this.render_pool_list(); }
},

async poolBatchDelete() {
  const ids = this.poolSelectedIds();
  if (!ids.length) return this.toast('请先勾选商品', 'err');
  if (!confirm('确定要删除 ' + ids.length + ' 件商品？')) return;
  for (const id of ids) {
    await this.postJSON('/pool/' + id, {}, 'DELETE');
  }
  this.toast('已删除', 'ok');
  this.render_pool_list();
},

async poolToggle(id, status) {
  const j = await this.postJSON('/pool/status', { ids: [id], status });
  if (j.code === 200) { this.toast('已更新', 'ok'); this.render_pool_list(); }
},

async poolDelete(id) {
  if (!confirm('确定要删除商品 #' + id + '？')) return;
  const j = await this.postJSON('/pool/' + id, {}, 'DELETE');
  if (j.code === 200) { this.toast('已删除', 'ok'); this.render_pool_list(); }
},

async editPool(id) {
  const j = await this.api('/pool?keyword=&page=1&limit=100');
  const item = (j.data || []).find(c => c.id === id);
  if (!item) return this.toast('未找到', 'err');
  const html = `
    <div class="card">
      <div class="card-head">
        <h2>编辑商品池 #${id}</h2>
        <button class="btn" onclick="AdminApp.render_pool()">返回列表</button>
      </div>
      <div class="form">
        <div class="field"><label>名称</label><input id="ep-name" value="${this.escapeHtml(item.name||'')}" /></div>
        <div class="field"><label>封面 URL</label><input id="ep-cover" value="${this.escapeHtml(item.cover||'')}" /></div>
        <div class="field"><label>描述 (HTML)</label><textarea id="ep-desc" rows="6">${this.escapeHtml(item.description||'')}</textarea></div>
        <div class="field"><label>售价</label><input id="ep-sale" type="number" step="0.01" value="${item.sale_price||item.price||0}" /></div>
        <div class="field"><label>成本</label><input id="ep-cost" type="number" step="0.01" value="${item.cost_price||0}" /></div>
        <div class="field"><label>库存</label><input id="ep-stock" type="number" value="${item.stock||0}" /></div>
        <div class="field"><label>起售</label><input id="ep-min" type="number" value="${item.minimum||1}" /></div>
        <div class="field"><label>限购</label><input id="ep-max" type="number" value="${item.maximum||0}" /></div>
        <div class="field"><label>排序</label><input id="ep-sort" type="number" value="${item.sort||0}" /></div>
        <div class="field"><label>标签 (逗号分隔)</label><input id="ep-tags" value="${this.escapeHtml(item.tags||'')}" /></div>
        <div class="field"><label>状态</label>
          <select id="ep-status">
            <option value="1" ${item.status===1?'selected':''}>上架</option>
            <option value="0" ${item.status===0?'selected':''}>下架</option>
          </select>
        </div>
        <button class="btn btn-primary" onclick="AdminApp.savePool(${id})">保存</button>
      </div>
    </div>
  `;
  document.getElementById('main').innerHTML = html;
},

async savePool(id) {
  const body = {
    name: document.getElementById('ep-name').value,
    cover: document.getElementById('ep-cover').value,
    description: document.getElementById('ep-desc').value,
    sale_price: parseFloat(document.getElementById('ep-sale').value) || 0,
    cost_price: parseFloat(document.getElementById('ep-cost').value) || 0,
    stock: parseInt(document.getElementById('ep-stock').value, 10) || 0,
    minimum: parseInt(document.getElementById('ep-min').value, 10) || 1,
    maximum: parseInt(document.getElementById('ep-max').value, 10) || 0,
    sort: parseInt(document.getElementById('ep-sort').value, 10) || 0,
    tags: document.getElementById('ep-tags').value,
    status: parseInt(document.getElementById('ep-status').value, 10),
  };
  const j = await this.putJSON('/pool/' + id, body);
  if (j.code === 200) { this.toast('已保存', 'ok'); this.render_pool(); }
},

async syncUpstream() {
  this.toast('同步中…', 'ok');
  const j = await this.postJSON('/pool/sync', {});
  if (j.code === 200) {
    const rs = j.data.results || [];
    const msg = rs.map(r => `${r.upstream}: 新增${r.added}/更新${r.updated}${r.error?' [错误]':''}`).join(' | ');
    this.toast(msg || '无变更', 'ok');
    this.render_pool();
  } else {
    this.toast('同步失败: ' + (j.msg||''), 'err');
  }
},
};

document.addEventListener('DOMContentLoaded', () => AdminApp.init());
