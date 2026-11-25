// Страница скачивания клиентских приложений (/apps) — Android, Windows, Linux.
import { router, api, h, ui, icon, fmt, state } from '../core.js';

const platIcon = p => p === 'android' ? 'smartphone' : 'monitor';
const isCreator = () => !!(state.user && (state.user.group_id === 15 || state.user.level >= 10));

router.add('/apps', async ctx => {
  ctx.title('Приложения');
  const d = await api.get('/api/apps');
  const items = d.items || [];
  const head = ui.pageHead('Приложения', d.version ? h('span.badge', 'v' + d.version) : null);

  if (!items.length) {
    const empty = ui.card(h('div.pad.stack',
      ui.empty('На данный момент приложения недоступны', 'smartphone'),
      isCreator() ? h('div.card.pad.stack-s', { style: { background: 'var(--surf-2)' } },
        h('b', 'Как собрать клиенты на сервере (видно только создателю):'),
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
      it.updated_at ? h('span', 'Обновлено: ', ui.time(it.updated_at)) : null,
      h('span.grow'),
      h('a.btn.primary.sm', { href: '/api/apps/download/' + encodeURIComponent(it.file), download: it.file }, icon('download', 15), ' Скачать'))));

  return [head, h('div.stack', ...cards)];
});
