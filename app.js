(() => {
  const els = {
    kpiGrid: document.getElementById('kpi-grid'),
    servicesGrid: document.getElementById('services-grid'),
    ticketsTbody: document.getElementById('tickets-tbody'),
    apiModePill: document.getElementById('api-mode-pill'),
    filterSev: document.getElementById('filter-sev'),
    filterStatus: document.getElementById('filter-status'),
    filterQ: document.getElementById('filter-q'),
    dialog: document.getElementById('ticket-dialog'),
    form: document.getElementById('ticket-form'),
  };

  let state = { stats: {}, services: [], tickets: [], liveApi: false };

  function esc(v) {
    return String(v ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function formatRemaining(deadlineStr, status) {
    if (status === 'resolved' || status === 'closed') return '已闭环达标';
    const diffMs = new Date(deadlineStr).getTime() - Date.now();
    if (Number.isNaN(diffMs)) return '-';
    if (diffMs <= 0) return `已超时 ${Math.abs(Math.floor(diffMs / 60000))}m`;
    const mins = Math.floor(diffMs / 60000);
    if (mins >= 60) return `剩余 ${Math.floor(mins / 60)}h ${mins % 60}m`;
    return `剩余 ${mins}m`;
  }

  async function refresh() {
    try {
      const qs = new URLSearchParams();
      if (els.filterSev.value) qs.set('severity', els.filterSev.value);
      if (els.filterStatus.value) qs.set('status', els.filterStatus.value);
      if (els.filterQ.value.trim()) qs.set('q', els.filterQ.value.trim());

      const [stRes, svcRes, tktRes] = await Promise.all([
        fetch('/api/stats'),
        fetch('/api/services'),
        fetch('/api/tickets?' + qs.toString()),
      ]);
      if (!stRes.ok || !svcRes.ok || !tktRes.ok) throw new Error('API unavailable');
      state.stats = await stRes.json();
      state.services = (await svcRes.json()).items || [];
      state.tickets = (await tktRes.json()).items || [];
      state.liveApi = true;
      els.apiModePill.textContent = 'Go 后端实时联调中';
    } catch (_) {
      state.liveApi = false;
      els.apiModePill.textContent = '静态预览模式';
    }
    render();
  }

  function render() {
    const s = state.stats;
    const kpis = [
      { label: '活跃告警工单', value: s.active_tickets ?? 0 },
      { label: 'P0 紧急未闭环', value: s.p0_active ?? 0 },
      { label: '排查推进中', value: s.investigating_count ?? 0 },
      { label: 'SLA 已违约', value: s.sla_breached ?? 0 },
      { label: '服务健康数', value: `${s.healthy_services ?? 0} / ${s.total_services ?? 0}` },
      { label: '累计已解决', value: s.resolved_count ?? 0 },
    ];
    els.kpiGrid.innerHTML = kpis.map((k) => `<div class="kpi"><span>${esc(k.label)}</span><strong>${esc(k.value)}</strong></div>`).join('');

    els.servicesGrid.innerHTML = state.services.map((svc) => `
      <article class="svc-card">
        <div class="svc-head">
          <strong>${esc(svc.name)}</strong>
          <span class="pill pill-${esc(svc.status)}">${esc(svc.status)}</span>
        </div>
        <div class="muted">负责人：${esc(svc.owner)} · 延迟 ${esc(svc.latency_ms)}ms · 可用性 ${esc(svc.availability)}%</div>
        <div class="actions">
          <button type="button" class="btn btn-ghost btn-sm" data-svc="${esc(svc.id)}" data-st="healthy">恢复健康</button>
          <button type="button" class="btn btn-ghost btn-sm" data-svc="${esc(svc.id)}" data-st="degraded">模拟降级</button>
          <button type="button" class="btn btn-ghost btn-sm" data-svc="${esc(svc.id)}" data-st="outage">模拟宕机</button>
        </div>
      </article>
    `).join('');

    els.servicesGrid.querySelectorAll('[data-svc]').forEach((btn) => {
      btn.addEventListener('click', async () => {
        await fetch(`/api/services/${btn.dataset.svc}/status`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ status: btn.dataset.st }),
        });
        await refresh();
      });
    });

    els.ticketsTbody.innerHTML = state.tickets.map((t) => {
      const rem = formatRemaining(t.sla_deadline, t.status);
      const breached = t.sla_breached;
      const nextActions = [];
      if (t.status === 'open') nextActions.push(['triaged', '分诊']);
      if (t.status === 'open' || t.status === 'triaged') nextActions.push(['investigating', '接手排查']);
      if (t.status !== 'resolved' && t.status !== 'closed') nextActions.push(['resolved', '标记解决']);
      if (t.status === 'resolved') nextActions.push(['closed', '关闭归档']);

      return `
        <tr>
          <th scope="row">${esc(t.id)}</th>
          <td data-label="级别"><span class="pill pill-${esc(t.severity)}">${esc(t.severity)}</span></td>
          <td data-label="故障标题"><strong>${esc(t.title)}</strong><div class="muted">${esc(t.description || '')}</div></td>
          <td data-label="关联服务">${esc(t.service_id)}</td>
          <td data-label="责任人">${esc(t.assignee)}</td>
          <td data-label="当前状态"><span class="pill pill-${esc(t.status)}">${esc(t.status)}</span></td>
          <td data-label="SLA 剩余"><span class="pill ${breached ? 'pill-breached' : ''}">${esc(rem)}</span></td>
          <td data-label="流转操作">
            <div class="actions">
              ${nextActions.map(([st, lbl]) => `<button type="button" class="btn btn-ghost btn-sm" data-tkt="${esc(t.id)}" data-next="${esc(st)}">${esc(lbl)}</button>`).join('')}
            </div>
          </td>
        </tr>`;
    }).join('');

    els.ticketsTbody.querySelectorAll('[data-tkt]').forEach((btn) => {
      btn.addEventListener('click', async () => {
        await fetch(`/api/tickets/${btn.dataset.tkt}/transition`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ status: btn.dataset.next, actor: 'OnCall-Console' }),
        });
        await refresh();
      });
    });
  }

  document.getElementById('btn-theme').addEventListener('click', () => {
    const next = document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', next);
    document.getElementById('btn-theme').textContent = next === 'dark' ? '浅色模式' : '深色模式';
  });

  document.getElementById('btn-new-ticket').addEventListener('click', () => els.dialog.showModal());
  document.getElementById('btn-cancel').addEventListener('click', () => els.dialog.close());
  els.dialog.addEventListener('close', () => { els.dialog.returnValue = ''; });
  els.dialog.addEventListener('click', (e) => { if (e.target === els.dialog) els.dialog.close(); });

  els.form.addEventListener('submit', async (e) => {
    e.preventDefault();
    await fetch('/api/tickets', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        title: document.getElementById('t-title').value.trim(),
        severity: document.getElementById('t-sev').value,
        service_id: document.getElementById('t-svc').value,
        assignee: document.getElementById('t-assignee').value.trim(),
        description: document.getElementById('t-desc').value.trim(),
      }),
    });
    els.form.reset();
    els.dialog.close();
    await refresh();
  });

  [els.filterSev, els.filterStatus].forEach((el) => el.addEventListener('change', refresh));
  els.filterQ.addEventListener('input', refresh);

  refresh();
  setInterval(refresh, 4000);
})();
