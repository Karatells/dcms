import { h, api, ui, router, state, icon, link, fmt } from '../core.js';

const me = () => state.user;
state.adminLinks.push({ group: 'Загрузки', title: 'Настройки загрузок', href: '/admin/loads', access: 'adm_set_loads', icon: 'folder', desc: 'Обменник: расширения, лимит баллов, новые файлы' });
const crumbs = (...c) => ui.breadcrumbs(c);
const fileHref = f => '/obmen/file/' + f.id;
const fname = (f, echo) => f.name + (echo ? '.' + f.ras : '');

const fileItem = (f, echo = 1) => h('a.list-item', { href: fileHref(f) },
  f.screen ? h('img.thumb', { src: f.screen_url, alt: '' }) : h('div.forum-ico', icon('file', 22)),
  h('div.main', h('div.title', fname(f, echo), f.metka ? h('span.badge.danger', { style: { marginLeft: '6px' } }, '18+') : null),
    f.opis ? h('div.small.muted.clip', f.opis) : null, h('div.small.muted', f.size_str, ' · ', f.owner.nick, ' · ', fmt.ago(f.time), f.dir ? ' · ' + f.dir.name : '', ' · ⬇ ', f.loads)),
  h('span.badge', icon('message', 12), ' ', f.komm));

const dirItem = d => h('a.list-item', { href: '/obmen/dir/' + d.id }, h('div.forum-ico', icon(d.my ? 'lock' : 'folder', 22)), h('div.main', h('div.title', d.name)),
  h('span.badge', d.files, d.new ? h('span.new', ' +' + d.new) : null));

function dirForm(d, onSubmit, submit) {
  const name = h('input.input', { value: d.name || '', maxlength: 64, placeholder: 'Название' });
  const ras = h('input.input', { value: d.ras || '', placeholder: 'Расширения через ; (jpg;png) или *' });
  const mb = h('input.input', { type: 'number', min: 0, value: d.maxsize ? Math.round(d.maxsize / 1048576) : 10, style: { width: '100px' } });
  const up = h('input', { type: 'checkbox', checked: !!d.upload });
  return h('div.stack', h('div.field', h('label', 'Название'), name), h('label.check', up, ' Разрешить выгрузку файлов'), h('div.field', h('label', 'Разрешённые расширения'), ras),
    h('div.row', 'Макс. размер (МБ):', mb), ui.btn(submit, { ic: 'check', kind: 'primary', onclick: () => onSubmit({ name: name.value, ras: ras.value, upload: up.checked, maxsize: Math.round((+mb.value || 0) * 1048576) }) }));
}

// ───────── обменник
router.add('/obmen', ctx => renderDir(ctx, 0));
router.add('/obmen/dir/:id', ctx => renderDir(ctx, +ctx.params.id));
async function renderDir(ctx, id) {
  const reload = () => router.render();
  const d = await api.get('/api/obmen/dir/' + id, { page: ctx.page, sort: ctx.query.sort });
  ctx.title(id ? 'Обменник — ' + d.name : 'Файловый обменник');
  const pList = Array.isArray(d.path) ? d.path : [];
  const path = [{ href: '/obmen', title: 'Обменник' }, ...pList.map((p, k) => k < pList.length - 1 ? { href: '/obmen/dir/' + p.id, title: p.name } : { title: p.name })];
  if (!id) path.pop(), path.push({ title: 'Обменник' });
  const out = [ui.breadcrumbs(path), ui.pageHead(id ? d.name : 'Файловый обменник', h('div.row', ui.btn('Поиск', { ic: 'search', small: true, href: '/obmen/search' }),
    me() && d.upload ? ui.btn('Добавить файл', { ic: 'upload', kind: 'primary', small: true, href: `/user/personalfiles/${me().id}/0/upload?obmen_dir=${id}` }) : null))];
  if (id && d.items.some(x => x.kind === 'file') && d.upload) out.push(h('div.chips', [['', 'Новые'], ['pop', 'Популярные']].map(([v, l]) => h('a.chip' + ((d.sort || '') === v ? '.active' : ''), { href: `/obmen/dir/${id}?sort=${v}` }, l))));
  out.push(ui.card(d.items.length ? h('div.list', d.items.map(x => x.kind === 'dir' ? dirItem(x) : fileItem(x, d.echo_ras))) : ui.empty('Нет результатов', 'folder')), ui.pager(d));
  if (d.upload) out.push(h('div.card.pad.small.muted', `Разрешено: ${d.ras || '—'} · размер до ${d.maxsize_str}`, d.limit ? ` · нужно баллов: ${d.limit}` : ''));
  if (d.can_create) out.push(h('details.card.pad', h('summary', icon('plus', 16), ' Создать папку'), dirForm({}, async v => { await api.post('/api/obmen/dir', { ...v, parent: id }); ui.ok('Папка создана'); reload(); }, 'Создать')));
  if (d.can_edit) {
    const tree = await api.get('/api/obmen/tree');
    const sel = h('select.input', h('option', { value: 0 }, '— корень —'), tree.filter(t => t.id !== id).map(t => h('option', { value: t.id, selected: t.id === d.parent }, '  '.repeat(t.depth) + t.name)));
    out.push(h('details.card.pad', h('summary', icon('settings', 16), ' Управление папкой'), h('div.stack', dirForm(d, async v => { await api.put('/api/obmen/dir/' + id, v); ui.ok('Сохранено'); reload(); }, 'Сохранить'),
      h('div.row', sel, ui.btn('Переместить', { onclick: async () => { await api.post(`/api/obmen/dir/${id}/move`, { parent: +sel.value }); ui.ok('Папка перемещена'); reload(); } })),
      d.can_delete ? ui.btn('Удалить папку с файлами', { ic: 'trash', kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить папку, вложенные папки и все файлы?', { danger: true })) { const r = await api.del('/api/obmen/dir/' + id); router.go(r.parent ? '/obmen/dir/' + r.parent : '/obmen'); } } }) : null)));
  }
  return out;
}

router.add('/obmen/search', async ctx => {
  ctx.title('Поиск файлов');
  const q = ctx.query.q || '';
  const d = q.length >= 2 ? await api.get('/api/obmen/search', { q, page: ctx.page }) : null;
  const input = h('input.input', { value: q, placeholder: 'Название или описание' });
  const go = () => router.go('/obmen/search?q=' + encodeURIComponent(input.value));
  input.onkeydown = e => e.key === 'Enter' && go();
  return [crumbs({ href: '/obmen', title: 'Обменник' }, { title: 'Поиск' }), ui.pageHead('Поиск файлов'), h('div.card.pad.row.nowrap', input, ui.btn('Найти', { ic: 'search', kind: 'primary', onclick: go })),
    d ? ui.card(d.items.length ? h('div.list', d.items.map(f => fileItem(f))) : ui.empty('Ничего не найдено', 'search')) : null, d ? ui.pager(d) : null];
});

// ───────── файл
router.add('/obmen/file/:id', async ctx => {
  const id = ctx.params.id;
  const reload = () => router.render();
  const d = await api.get('/api/obmen/file/' + id, { page: ctx.query.page || ctx.page, sort: ctx.query.sort });
  ctx.title(d.name);
  const path = [{ href: '/id' + d.owner.id, title: d.owner.nick }, { href: `/user/personalfiles/${d.owner.id}/0`, title: 'Файлы' }, ...d.folder.path.slice(1).map(p => ({ href: `/user/personalfiles/${d.owner.id}/${p.id}`, title: p.name })), { title: d.name }];
  const head = [ui.breadcrumbs(path), ui.pageHead(d.name + (d.echo_ras ? '.' + d.ras : ''), d.metka ? h('span.badge.danger', '18+') : null)];
  if (d.blocked) {
    const p = h('input.input', { type: 'password', placeholder: 'Пароль' });
    const go = async () => { await api.post(`/api/pfiles/folder/${d.folder_id}/unlock`, { password: p.value }); reload(); };
    return [...head, h('div.card.pad.stack', h('div', icon('lock', 16), ' Папка защищена паролем'), p, ui.btn('Войти', { kind: 'primary', onclick: go }))];
  }
  if (d.adult) return [...head, d.need_login ? h('div.card.pad', 'Файл 18+ доступен только зарегистрированным. ', link('/login', 'Войти')) : h('div.card.pad.stack', h('div', icon('alert', 16), ' Файл содержит материалы эротического характера. Вам есть 18 лет?'), ui.btn('Да, продолжить', { kind: 'primary', onclick: async () => { await api.post('/api/obmen/abuld'); reload(); } }))];
  const out = [...head];
  const media = d.kind === 'audio' ? h('audio', { controls: true, src: d.download + '?inline=1', style: { width: '100%' } }) : d.kind === 'video' ? h('video', { controls: true, src: d.download + '?inline=1', style: { maxWidth: '100%', borderRadius: '12px' } }) : d.kind === 'image' ? h('img', { src: d.download + '?inline=1', style: { maxWidth: '100%', maxHeight: '60vh', borderRadius: '12px' } }) : d.screen ? h('img', { src: d.screen_url, style: { maxWidth: '100%', borderRadius: '12px' } }) : null;
  out.push(h('div.card.pad.stack', media, d.opis ? h('div', { html: d.opis }) : h('div.muted', 'Без описания'),
    h('div.small.muted', `Размер: ${d.size_str} · Выгрузил: `, ui.userLink(d.owner), ' · ', fmt.datetime(d.time), ` · Скачано: ${d.loads}`, d.dir.my ? '' : [' · Зона обмена: ', link('/obmen/dir/' + d.dir.id, d.dir.name)]),
    h('div.row', ui.btn('Скачать (' + d.size_str + ')', { ic: 'download', kind: 'primary', href: d.download }),
      me() ? ui.btn(d.bookmarked ? 'Из закладок' : 'В закладки', { ic: 'star', small: true, kind: d.bookmarked ? 'primary' : undefined, onclick: async () => { await api.post('/api/bookmark/toggle', { type: 'file', id: +id }); reload(); } }) : null,
      d.music ? ui.btn(d.music.in ? 'Из плейлиста' : 'В плейлист (' + d.music.people + ')', { ic: 'music', small: true, kind: d.music.in ? 'primary' : undefined, onclick: async () => { await api.post('/api/music/toggle/' + id); reload(); } }) : null,
      h('span.grow'), ui.btn(String(d.likes), { ic: 'thumbs-up', small: true, kind: d.my_like === 1 ? 'primary' : undefined, onclick: me() && !d.mine && d.my_like < 0 ? async () => { await api.post(`/api/obmen/file/${id}/like`, { like: true }); reload(); } : undefined }),
      ui.btn(String(d.dislikes), { ic: 'thumbs-down', small: true, kind: d.my_like === 0 ? 'primary' : undefined, onclick: me() && !d.mine && d.my_like < 0 ? async () => { await api.post(`/api/obmen/file/${id}/like`, { like: false }); reload(); } : undefined }))));
  const acts = [];
  if (d.mine) {
    acts.push(ui.btn(d.dir.my ? 'В зону обмена' : 'Сменить папку обмена', { ic: 'folder', small: true, onclick: () => publish(d, reload) }));
    if (!d.dir.my) acts.push(ui.btn('Снять с публикации', { small: true, onclick: async () => { await api.post(`/api/obmen/file/${id}/publish`, { dir: 0 }); reload(); } }));
    acts.push(ui.btn('В другую папку', { ic: 'folder', small: true, onclick: () => moveFolder(d, reload) }));
  }
  if (d.can_delete) acts.push(ui.btn('Удалить', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить файл?', { danger: true })) { const r = await api.del('/api/obmen/file/' + id); ui.ok('Файл удалён'); router.go(`/user/personalfiles/${r.owner}/${r.folder}`); } } }));
  if (acts.length) out.push(h('div.row', ...acts, h('span.grow'), d.newer ? link('/obmen/file/' + d.newer, '← Новее') : null, d.older ? link('/obmen/file/' + d.older, 'Старее →') : null));
  if (d.can_edit) {
    const name = h('input.input', { value: d.name, maxlength: 128 }), opis = h('textarea.input', { rows: 3 }, d.raw_opis || ''), metka = h('input', { type: 'checkbox', checked: d.metka });
    out.push(h('details.card.pad', h('summary', icon('edit', 16), ' Редактировать'), h('div.stack', h('div.field', h('label', 'Название'), name), h('div.field', h('label', 'Описание'), opis), h('label.check', metka, ' Метка 18+'),
      ui.btn('Сохранить', { ic: 'check', kind: 'primary', onclick: async () => { await api.put('/api/obmen/file/' + id, { name: name.value, opis: opis.value, metka: metka.checked }); ui.ok('Файл отредактирован'); reload(); } }))));
  }
  const k = d.komms;
  const ed = ui.bbEditor({ rows: 3, placeholder: 'Комментарий', maxlength: 1024 });
  let replyTo = 0;
  const answer = p => { replyTo = p.user.id; ed.ta.value = `[b]${p.user.nick}[/b], `; ed.ta.focus(); };
  out.push(h('h3', 'Комментарии (' + k.total + ')'));
  if (me() && k.total) out.push(h('div.chips', ['Вверху', 'Внизу'].map((l, v) => h('a.chip' + (d.sort === v ? '.active' : ''), { href: `/obmen/file/${id}?sort=${v}` }, l))));
  out.push(ui.card(k.items.length ? h('div.posts', k.items.map(p => ui.post({ user: p.user, time: p.time, body: h('div.post-body', { html: p.msg }), id: 'k' + p.id, actions: [
    p.can_reply ? ui.btn('', { ic: 'corner-up-left', small: true, onclick: () => answer(p) }) : null,
    p.can_spam ? ui.btn('', { ic: 'flag', small: true, title: 'Пожаловаться', onclick: async () => { const t = await ui.prompt('Опишите нарушение', '', { title: 'Жалоба', multiline: true }); if (t) { await api.post(`/api/obmen-komm/${p.id}/spam`, { msg: t, types: 0 }); ui.ok('Жалоба отправлена'); } } }) : null,
    p.can_delete ? ui.btn('', { ic: 'trash', small: true, onclick: async () => { if (await ui.confirm('Удалить комментарий?', { danger: true })) { await api.del('/api/obmen-komm/' + p.id); reload(); } } }) : null].filter(Boolean) }))) : ui.empty('Нет комментариев', 'message')), ui.pager(k));
  out.push(me() ? h('div.card.pad.stack', ed.el, h('div.row', h('span.grow'), ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: async () => { await api.post(`/api/obmen/file/${id}/komm`, { msg: ed.value, reply_to: replyTo }); ui.ok('Сообщение добавлено'); reload(); } }))) : h('div.card.pad.muted', link('/login', 'Войдите'), ', чтобы комментировать.'));
  return out;
});

async function publish(d, reload) {
  const tree = (await api.get('/api/obmen/tree')).filter(t => t.upload);
  const sel = h('select.input', tree.map(t => h('option', { value: t.id }, '  '.repeat(t.depth) + t.name + ` (${t.ras})`)));
  ui.modal({ title: 'Выложить в зону обмена', body: sel, actions: [{ label: 'Отмена' }, { label: 'Выложить', kind: 'primary', onclick: () => { api.post(`/api/obmen/file/${d.id}/publish`, { dir: +sel.value }).then(() => { ui.ok('Файл добавлен в зону обмена'); reload(); }); } }] });
}
async function moveFolder(d, reload) {
  const all = [];
  const walk = async fid => { const r = await api.get(`/api/pfiles/${d.owner.id}/${fid}`, { page: 1 }); all.push({ id: r.id, name: r.name, depth: r.osn - 1 }); for (const x of r.items.filter(i => i.kind === 'dir' && !i.lock)) await walk(x.id); };
  await walk(0);
  const sel = h('select.input', all.map(t => h('option', { value: t.id, selected: t.id === d.folder.id }, '  '.repeat(t.depth) + t.name)));
  ui.modal({ title: 'Переместить файл', body: sel, actions: [{ label: 'Отмена' }, { label: 'Переместить', kind: 'primary', onclick: () => { api.post(`/api/obmen/file/${d.id}/move`, { folder: +sel.value }).then(() => { ui.ok('Файл перемещён'); reload(); }); } }] });
}

// ───────── личные файлы
router.add('/user/personalfiles/:uid/:dir/upload', async ctx => {
  const { uid, dir } = ctx.params;
  ctx.title('Загрузка файла');
  const root = await api.get(`/api/pfiles/${uid}/${dir}`);
  const tree = (await api.get('/api/obmen/tree')).filter(t => t.upload);
  const sel = h('select.input', h('option', { value: 0 }, 'Только в личную папку'), tree.map(t => h('option', { value: t.id, selected: String(t.id) === ctx.query.obmen_dir }, '  '.repeat(t.depth) + t.name + ` (${t.ras}, до ${Math.round(t.maxsize / 1048576)} МБ)`)));
  const file = h('input', { type: 'file' }), screen = h('input', { type: 'file', accept: 'image/*' });
  const name = h('input.input', { maxlength: 128, placeholder: 'Название (по умолчанию — имя файла)' }), msg = h('textarea.input', { rows: 3, placeholder: 'Описание' }), metka = h('input', { type: 'checkbox' });
  const send = async () => {
    if (!file.files[0]) return ui.toast('Выберите файл', 'warn');
    const r = await api.upload('/api/pfiles/upload', { folder: root.id, obmen_dir: sel.value, file: file.files[0], screen: screen.files[0] || '', name: name.value, msg: msg.value, metka: metka.checked ? '1' : '0' });
    ui.ok('Файл выгружен'); router.go('/obmen/file/' + r.id);
  };
  return [crumbs({ href: `/user/personalfiles/${uid}/0`, title: 'Файлы' }, { title: 'Загрузка' }), ui.pageHead('Загрузка файла в «' + root.name + '»'),
    h('div.card.pad.stack', h('div.field', h('label', 'Файл'), file), h('div.field', h('label', 'Название'), name), h('div.field', h('label', 'Описание'), msg), h('div.field', h('label', 'Скриншот / обложка (необязательно)'), screen),
      h('div.field', h('label', 'Куда выложить'), sel), h('label.check', metka, ' Метка ', h('b.danger-text', '18+')), ui.btn('Загрузить', { ic: 'upload', kind: 'primary', onclick: send }))];
}, { auth: true });

const pf = async ctx => {
  const { uid } = ctx.params, dir = ctx.params.dir || 0;
  const reload = () => router.render();
  const d = await api.get(`/api/pfiles/${uid}/${dir}`, { page: ctx.page });
  ctx.title(d.name + ' — ' + d.owner.nick);
  const head = [ui.breadcrumbs([{ href: '/id' + d.owner.id, title: d.owner.nick }, ...d.path.map((p, k) => k < d.path.length - 1 ? { href: `/user/personalfiles/${uid}/${p.id}`, title: p.name } : { title: p.name })]), ui.pageHead(d.name, d.mine ? ui.btn('Загрузить файл', { ic: 'upload', kind: 'primary', small: true, href: `/user/personalfiles/${uid}/${d.id}/upload` }) : null)];
  if (d.blocked) {
    const p = h('input.input', { type: 'password', placeholder: 'Пароль' });
    const go = async () => { await api.post(`/api/pfiles/folder/${d.id}/unlock`, { password: p.value }); reload(); };
    return [...head, h('div.card.pad.stack', h('div', icon('lock', 16), ' Папка защищена паролем'), p, ui.btn('Войти', { kind: 'primary', onclick: go }))];
  }
  const out = [...head, d.msg ? h('div.muted', { html: d.msg }) : null];
  out.push(ui.card(d.items.length ? h('div.list', d.items.map(x => x.kind === 'dir'
    ? h('div.list-item', h('a.row.nowrap.grow', { href: `/user/personalfiles/${uid}/${x.id}` }, h('div.forum-ico', icon(x.lock ? 'lock' : 'folder', 22)), h('div.main', h('div.title', x.name), x.msg ? h('div.small.muted', x.msg) : null)), h('span.badge', `${x.folders}/${x.files}`),
      x.can_edit ? h('span.row', ui.btn('', { ic: 'edit', small: true, onclick: () => editFolder(x.id, reload) }), ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm(`Удалить папку «${x.name}» со всем содержимым?`, { danger: true })) { const r = await api.del('/api/pfiles/folder/' + x.id); ui.ok(`Удалено папок: ${r.folders}, файлов: ${r.files}`); reload(); } } })) : null)
    : fileItem(x))) : ui.empty('Папка пуста', 'folder')), ui.pager(d));
  if (d.can_create) out.push(h('details.card.pad', h('summary', icon('plus', 16), ' Создать папку'), folderForm({}, async v => { await api.post('/api/pfiles/folder', { ...v, parent: d.id }); ui.ok('Папка создана'); reload(); }, 'Создать')));
  if (d.can_edit && d.osn > 1) out.push(h('details.card.pad', h('summary', icon('settings', 16), ' Настройки этой папки'), folderForm({ name: d.name, msg: d.raw_msg, pass: d.pass }, async v => { await api.put('/api/pfiles/folder/' + d.id, v); ui.ok('Сохранено'); reload(); }, 'Сохранить')));
  return out;
};
router.add('/user/personalfiles/:uid', pf);
router.add('/user/personalfiles/:uid/:dir', pf);

function folderForm(f, onSubmit, submit) {
  const name = h('input.input', { value: f.name || '', maxlength: 30, placeholder: 'Название' }), msg = h('textarea.input', { rows: 2, maxlength: 256, placeholder: 'Описание' }, f.msg || ''), pass = h('input.input', { value: f.pass || '', maxlength: 12, placeholder: 'Пароль (необязательно)' });
  return h('div.stack', name, msg, pass, ui.btn(submit, { ic: 'check', kind: 'primary', onclick: () => onSubmit({ name: name.value, msg: msg.value, pass: pass.value }) }));
}
async function editFolder(id, reload) {
  const r = await api.get(`/api/pfiles/${state.user.id}/${id}`);
  const box = h('div'); const form = folderForm({ name: r.name, msg: r.raw_msg, pass: r.pass }, async v => { await api.put('/api/pfiles/folder/' + id, v); ui.ok('Сохранено'); reload(); }, 'Сохранить');
  box.append(form); ui.modal({ title: 'Редактирование папки', body: box, actions: [{ label: 'Закрыть' }] });
}

// ───────── админка
router.add('/admin/loads', async ctx => {
  ctx.title('Настройки загрузок');
  const s = await api.get('/api/admin/loads/settings');
  return [ui.pageHead('Настройки загрузок'), ui.card(h('div.pad', ui.form([
    { name: 'echo_rassh', label: 'Показывать расширения файлов', type: 'select', value: String(s.echo_rassh), options: [{ value: '1', label: 'Да' }, { value: '0', label: 'Нет' }] },
    { name: 'loads_new_file_hour', label: 'Время, в течение которого файл считается новым (часы)', type: 'number', value: s.loads_new_file_hour },
    { name: 'obmen_limit_up', label: 'Минимум баллов для выгрузки в зону обмена', type: 'number', value: s.obmen_limit_up }],
    { submit: 'Сохранить', onSubmit: async v => { await api.put('/api/admin/loads/settings', { echo_rassh: +v.echo_rassh, loads_new_file_hour: +v.loads_new_file_hour, obmen_limit_up: +v.obmen_limit_up }); ui.ok('Настройки сохранены'); } })))];
}, { auth: true, access: 'adm_set_loads' });
