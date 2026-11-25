// Страница скачивания клиентских приложений (/apps) — Android, Windows, Linux.
import { router, api, h, ui, icon, fmt, can } from '../core.js';

const platIcon = p => p === 'android' ? 'smartphone' : 'monitor';

router.add('/apps', async ctx => {
  ctx.title('Приложения');
  const d = await api.get('/api/apps');
  const items = d.items || [];
  const head = ui.pageHead('Приложения', d.version ? h('span.badge', 'v' + d.version) : null);

  if (!items.length) {
    const empty = ui.card(h('div.pad.stack',
      ui.empty('Собранные клиенты пока не опубликованы', 'smartphone'),
      can('adm_panel_show') ? h('div.card.pad.stack-s', { style: { background: 'var(--surf-2)' } },
        h('b', 'Как собрать клиенты на сервере:'),
        h('div.small.muted', 'Запустите на сервере скрипт сборки (поддерживаются x86_64 и arm64):'),
        h('pre', { style: { margin: 0, overflowX: 'auto', fontSize: '.82rem' } },
          `sudo dcms build-clients --url ${location.origin} --name "${d.app_name || 'DCMS Social'}"`)) : null));
    return [head, empty];
  }

  const cards = items.map(it => h('div.card.pad.stack-s',
    h('div.row',
      h('span.logo-mark', icon(platIcon(it.platform), 18)),
      h('div.grow',
        h('div', { style: { fontWeight: 700, fontSize: '1.02rem' } }, it.label || it.file),
        h('div.small.muted', it.note || '')),
      h('span.badge', fmt.size(it.size || 0))),
    h('div.row.small.muted',
      it.updated_at ? h('span', 'Собрано: ', ui.time(it.updated_at)) : null,
      h('span.grow'),
      h('a.btn.primary.sm', { href: '/api/apps/download/' + encodeURIComponent(it.file), download: it.file }, icon('download', 15), ' Скачать')),
    it.sha256 ? h('div.small.muted', { style: { fontFamily: 'monospace', fontSize: '.72rem', wordBreak: 'break-all', opacity: .75 } }, 'SHA-256: ' + it.sha256) : null));

  const info = h('div.card.pad.small.muted.row',
    icon('globe', 15),
    h('span', 'Сервер: ', h('b', d.server_url || location.origin), d.rev ? ` · ревизия ${d.rev}` : ''));

  return [head, info, h('div.stack', ...cards)];
});
