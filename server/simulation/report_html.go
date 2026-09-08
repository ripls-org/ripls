package simulation

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// WriteReportHTML renders the report as a self-contained HTML file.
func WriteReportHTML(w io.Writer, report *Report) error {
	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	tmpl, err := template.New("report").Parse(reportTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	//nolint:gosec // G203: `data` is JSON this package just marshalled from its own
	// simulation results, and the report is a local dev artifact — never served.
	return tmpl.Execute(w, template.JS(data))
}

const reportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Simulation Report</title>
<style>
  :root {
    --bg: #f8f9fa;
    --card-bg: #fff;
    --text: #212529;
    --text-muted: #6c757d;
    --border: #dee2e6;
    --gear: #4dabf7;
    --loans: #51cf66;
    --giveaways: #9775fa;
    --requests: #fcc419;
    --experiences: #ff922b;
    --cancellations: #ff6b6b;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: var(--bg);
    color: var(--text);
    padding: 24px;
    max-width: 1200px;
    margin: 0 auto;
  }
  h1 { font-size: 1.75rem; margin-bottom: 4px; }
  h2 { font-size: 1.25rem; margin: 24px 0 12px; }
  .subtitle { color: var(--text-muted); margin-bottom: 24px; }
  .meta { display: flex; gap: 24px; flex-wrap: wrap; color: var(--text-muted); font-size: 0.875rem; margin-bottom: 16px; }
  .meta span { white-space: nowrap; }

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 12px;
    margin-bottom: 24px;
  }
  .card {
    background: var(--card-bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
    text-align: center;
  }
  .card .value { font-size: 2rem; font-weight: 700; }
  .card .label { font-size: 0.8rem; color: var(--text-muted); margin-top: 4px; }

  .chart-container { margin-bottom: 32px; }
  .bar-row {
    display: flex;
    align-items: center;
    margin-bottom: 4px;
    font-size: 0.75rem;
  }
  .bar-label {
    width: 80px;
    flex-shrink: 0;
    text-align: right;
    padding-right: 8px;
    color: var(--text-muted);
  }
  .bar-track {
    flex: 1;
    height: 22px;
    display: flex;
    border-radius: 3px;
    overflow: hidden;
    background: #e9ecef;
  }
  .bar-seg {
    height: 100%;
    transition: width 0.3s;
  }
  .bar-total {
    width: 40px;
    text-align: right;
    padding-left: 6px;
    color: var(--text-muted);
    font-size: 0.75rem;
  }

  .legend {
    display: flex;
    gap: 16px;
    flex-wrap: wrap;
    margin-bottom: 12px;
    font-size: 0.8rem;
  }
  .legend-item { display: flex; align-items: center; gap: 4px; }
  .legend-swatch {
    width: 12px;
    height: 12px;
    border-radius: 2px;
    flex-shrink: 0;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
    margin-bottom: 32px;
  }
  th, td {
    padding: 8px 12px;
    text-align: left;
    border-bottom: 1px solid var(--border);
  }
  th {
    background: var(--card-bg);
    font-weight: 600;
    position: sticky;
    top: 0;
    cursor: pointer;
    user-select: none;
  }
  th:hover { background: #e9ecef; }
  td.num { text-align: right; font-variant-numeric: tabular-nums; }
  th.num { text-align: right; }
  tr:hover td { background: #f1f3f5; }

  .persona-badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 10px;
    font-size: 0.75rem;
    font-weight: 500;
  }
  .persona-alfred { background: #d0ebff; color: #1864ab; }
  .persona-derek { background: #d3f9d8; color: #2b8a3e; }
  .persona-gary { background: #e5dbff; color: #5f3dc4; }
  .persona-betty { background: #fff3bf; color: #e67700; }
  .persona-emma { background: #ffe8cc; color: #d9480f; }
  .persona-henry { background: #ffe3e3; color: #c92a2a; }

  .creds-section { margin-top: 24px; }
  .creds-section .pw {
    background: #e9ecef;
    padding: 4px 8px;
    border-radius: 4px;
    font-family: monospace;
    font-size: 0.85rem;
  }

  @media (max-width: 600px) {
    body { padding: 12px; }
    .cards { grid-template-columns: repeat(2, 1fr); }
    .bar-label { width: 50px; font-size: 0.65rem; }
    table { font-size: 0.75rem; }
    th, td { padding: 6px 8px; }
  }
</style>
</head>
<body>

<div id="app"></div>

<script>
const R = {{.}};

const $ = (s, p) => (p||document).querySelector(s);
const h = (tag, attrs, ...children) => {
  const el = document.createElement(tag);
  if (attrs) Object.entries(attrs).forEach(([k,v]) => {
    if (k === 'className') el.className = v;
    else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
    else if (k.startsWith('on')) el.addEventListener(k.slice(2).toLowerCase(), v);
    else el.setAttribute(k, v);
  });
  children.flat().forEach(c => {
    if (c == null) return;
    el.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
  });
  return el;
};

const colors = {
  gear: 'var(--gear)',
  loans: 'var(--loans)',
  giveaways: 'var(--giveaways)',
  requests: 'var(--requests)',
  experiences: 'var(--experiences)',
  cancellations: 'var(--cancellations)'
};

const categories = ['gear','loans','giveaways','requests','experiences','cancellations'];
const catLabels = {gear:'Gear Setup',loans:'Loans',giveaways:'Giveaways',requests:'Requests',experiences:'Experiences',cancellations:'Cancellations'};

function personaClass(p) {
  return 'persona-badge persona-' + (p||'').toLowerCase();
}

function renderHeader(app) {
  app.appendChild(h('h1', null, 'Simulation Report: ' + R.scenario_name));
  app.appendChild(h('p', {className:'subtitle'}, R.description));
  const dur = (R.duration_ms / 1000).toFixed(1);
  app.appendChild(h('div', {className:'meta'},
    h('span', null, 'Period: ' + R.start_date + ' → ' + R.end_date),
    h('span', null, 'Wall time: ' + dur + 's'),
    h('span', null, 'Seed: ' + R.seed),
    h('span', null, 'Steps: ' + R.summary.timeline_steps)
  ));
}

function renderSummary(app) {
  const s = R.summary;
  const items = [
    [s.users, 'Users'],
    [s.communities, 'Communities'],
    [s.gear, 'Gear Items'],
    [s.transfers_started, 'Transfers Started'],
    [s.transfers_completed, 'Transfers Completed'],
    [s.transfers_cancelled, 'Transfers Cancelled'],
    [s.requests_submitted, 'Requests Submitted'],
    [s.requests_fulfilled, 'Requests Fulfilled'],
    [s.requests_cancelled, 'Requests Cancelled'],
    [s.experiences_created, 'Experiences Created'],
    [s.experiences_completed, 'Experiences Completed'],
    [s.experiences_cancelled, 'Experiences Cancelled'],
    [s.rsvp_yes, 'RSVPs (Yes)'],
    [s.rsvp_no, 'RSVPs (No)'],
    [s.chat_messages_sent, 'Chat Messages'],
    [s.steps_skipped, 'Steps Skipped'],
    [s.steps_failed, 'Steps Failed'],
  ];
  const grid = h('div', {className:'cards'});
  items.forEach(([v,l]) => {
    grid.appendChild(h('div', {className:'card'},
      h('div', {className:'value'}, String(v)),
      h('div', {className:'label'}, l)
    ));
  });
  app.appendChild(grid);
}

function renderChart(app) {
  const buckets = R.weekly_buckets || [];
  if (!buckets.length) return;

  app.appendChild(h('h2', null, 'Weekly Activity'));

  const legend = h('div', {className:'legend'});
  categories.forEach(c => {
    legend.appendChild(h('div', {className:'legend-item'},
      h('div', {className:'legend-swatch', style:{background:colors[c]}}),
      catLabels[c]
    ));
  });
  app.appendChild(legend);

  const maxTotal = Math.max(...buckets.map(b =>
    categories.reduce((s,c) => s + (b[c]||0), 0)
  ), 1);

  const container = h('div', {className:'chart-container'});
  buckets.forEach(b => {
    const total = categories.reduce((s,c) => s + (b[c]||0), 0);
    const track = h('div', {className:'bar-track'});
    categories.forEach(c => {
      const v = b[c] || 0;
      if (v > 0) {
        const pct = (v / maxTotal * 100).toFixed(1) + '%';
        track.appendChild(h('div', {
          className:'bar-seg',
          style:{width:pct, background:colors[c]},
          title: catLabels[c] + ': ' + v
        }));
      }
    });
    const label = b.week_start.slice(5);
    container.appendChild(h('div', {className:'bar-row'},
      h('div', {className:'bar-label'}, label),
      track,
      h('div', {className:'bar-total'}, String(total))
    ));
  });
  app.appendChild(container);
}

function renderUserTable(app) {
  const rows = R.user_activity || [];
  if (!rows.length) return;

  app.appendChild(h('h2', null, 'Per-User Activity'));

  const cols = [
    {key:'name', label:'Name', num:false},
    {key:'persona', label:'Persona', num:false},
    {key:'community', label:'Community', num:false},
    {key:'gear', label:'Gear', num:true},
    {key:'loans', label:'Loans', num:true},
    {key:'giveaways', label:'Give', num:true},
    {key:'requests', label:'Req', num:true},
    {key:'experiences', label:'Exp', num:true},
    {key:'cancellations', label:'Cancel', num:true},
    {key:'total', label:'Total', num:true},
  ];

  let sortKey = 'total';
  let sortAsc = false;
  let sorted = [...rows];

  function doSort() {
    sorted.sort((a,b) => {
      let va = a[sortKey], vb = b[sortKey];
      if (typeof va === 'string') { va = va.toLowerCase(); vb = vb.toLowerCase(); }
      if (va < vb) return sortAsc ? -1 : 1;
      if (va > vb) return sortAsc ? 1 : -1;
      return 0;
    });
  }

  const table = h('table');
  const thead = h('thead');
  const headerRow = h('tr');
  cols.forEach(c => {
    const th_ = h('th', {className: c.num ? 'num' : '', onClick: () => {
      if (sortKey === c.key) sortAsc = !sortAsc;
      else { sortKey = c.key; sortAsc = c.num ? false : true; }
      doSort();
      renderBody();
    }}, c.label);
    headerRow.appendChild(th_);
  });
  thead.appendChild(headerRow);
  table.appendChild(thead);
  const tbody = h('tbody');
  table.appendChild(tbody);

  function renderBody() {
    tbody.innerHTML = '';
    sorted.forEach(r => {
      const tr = h('tr');
      cols.forEach(c => {
        if (c.key === 'persona') {
          tr.appendChild(h('td', null, h('span', {className:personaClass(r.persona)}, r.persona)));
        } else {
          tr.appendChild(h('td', {className: c.num ? 'num' : ''}, String(r[c.key])));
        }
      });
      tbody.appendChild(tr);
    });
  }

  doSort();
  renderBody();
  app.appendChild(table);
}

function renderCommunityTable(app) {
  const rows = R.community_breakdown || [];
  if (!rows.length || rows.length < 2) return;

  app.appendChild(h('h2', null, 'Community Breakdown'));

  const table = h('table');
  const cols = ['Name','Members','Gear','Loans','Giveaways','Requests','Experiences','Cancellations','Total'];
  const keys = ['name','members','gear','loans','giveaways','requests','experiences','cancellations','total'];
  const tr = h('tr');
  cols.forEach((c,i) => tr.appendChild(h('th', {className: i>0?'num':''}, c)));
  table.appendChild(h('thead', null, tr));
  const tbody = h('tbody');
  rows.forEach(r => {
    const row = h('tr');
    keys.forEach((k,i) => row.appendChild(h('td', {className:i>0?'num':''}, String(r[k]))));
    tbody.appendChild(row);
  });
  table.appendChild(tbody);
  app.appendChild(table);
}

function renderCredentials(app) {
  const creds = R.credentials || [];
  if (!creds.length) return;

  const section = h('div', {className:'creds-section'});
  section.appendChild(h('h2', null, 'Simulated Accounts'));
  section.appendChild(h('p', null,
    'These accounts have no password. Sign in as one by requesting an emailed ',
    'code for its address and reading it from the dev-mode echo on ',
    h('span', {className:'pw'}, 'RequestEmailCode'),
    '. No mail is sent \u2014 example.com is a reserved test domain.'
  ));

  const table = h('table');
  const tr = h('tr');
  ['Name','Email','Persona','Community'].forEach(c => tr.appendChild(h('th', null, c)));
  table.appendChild(h('thead', null, tr));
  const tbody = h('tbody');
  creds.forEach(c => {
    tbody.appendChild(h('tr', null,
      h('td', null, c.name),
      h('td', null, c.email),
      h('td', null, h('span', {className:personaClass(c.persona)}, c.persona)),
      h('td', null, c.community)
    ));
  });
  table.appendChild(tbody);
  section.appendChild(table);
  app.appendChild(section);
}

const app = $('#app');
renderHeader(app);
renderSummary(app);
renderChart(app);
renderUserTable(app);
renderCommunityTable(app);
renderCredentials(app);
</script>
</body>
</html>`
