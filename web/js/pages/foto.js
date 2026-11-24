import { h, api, ui, router, state, icon, link, fmt } from '../core.js';

const me = () => state.user;
state.adminLinks.push({ group: 'Фотогалерея', title: 'Настройки фотогалереи', href: '/admin/foto', access: 'adm_set_foto', icon: 'camera', desc: 'Максимальные размеры загружаемых фото' });

const crumbs = (...c) => ui.breadcrumbs(c);
const albumHref = (a, uid) => `/foto/${uid ?? a.owner.id}/${a.id}`;
const radioRow = (label, name, val, onchange) => h('div.field', h('label', label), h('div.chips', [['0', 'Все'], ['1', 'Друзья'], ['2', 'Только я']].map(([v, l]) => {
  const r = h('input', { type: 'radio', name, value: v, checked: String(val) === v, onchange: () => onchange(+v) });
  return h('label.chip', r, ' ', l);
})));

function albumForm(a, onSubmit, submit) {
  const st = { privat: a.privat ?? 0, privat_komm: a.privat_komm ?? 0 };
  const name = h('input.input', { value: a.name || '', maxlength: 32, placeholder: 'Название альбома' });
  const opis = h('textarea.input', { rows: 3, maxlength: 256, placeholder: 'Описание' }, a.raw_opis || '');
  const pass = h('input.input', { value: a.pass || '', maxlength: 11, placeholder: 'Пароль (необязательно)' });
  return h('div.stack', h('div.field', h('label', 'Название'), name), h('div.field', h('label', 'Описание'), opis), h('div.field', h('label', 'Пароль'), pass),
    radioRow('Могут смотреть', 'privat' + (a.id || 0), st.privat, v => (st.privat = v)), radioRow('Могут комментировать', 'pk' + (a.id || 0), st.privat_komm, v => (st.privat_komm = v)),
    ui.btn(submit, { ic: 'check', kind: 'primary', onclick: () => onSubmit({ name: name.value, opis: opis.value, pass: pass.value, ...st }) }));
}

const albumTile = (al, edit) => h('a.album-tile', { href: albumHref(al) },
  h('div.album-cover', al.cover ? h('img', { src: al.cover, alt: '', loading: 'lazy' }) : icon('camera', 36), al.locked ? h('span.album-lock', icon('lock', 14)) : null),
  h('div.album-info', h('div.title', al.name), h('div.small.muted', al.owner ? al.owner.nick + ' · ' : '', `${al.count} фото`)));

// ───────── список всех альбомов
router.add('/foto', async ctx => {
  ctx.title('Фотоальбомы');
  const d = await api.get('/api/foto', { page: ctx.page });
  return [ui.pageHead('Фотоальбомы', me() ? h('div.row', ui.btn('Мои альбомы', { ic: 'image', href: '/foto/user/' + me().id }), ui.btn('Создать альбом', { ic: 'plus', kind: 'primary', href: '/foto/user/' + me().id + '?create=1' })) : null),
    d.items.length ? h('div.album-grid', d.items.map(al => albumTile(al))) : ui.card(ui.empty('Нет фотоальбомов', 'camera')), ui.pager(d),
    h('div.card.pad.small.muted', icon('camera', 14), ` Всего фотографий: ${d.photos}`)];
});

// ───────── альбомы пользователя
router.add('/foto/user/:uid', async ctx => {
  const d = await api.get('/api/foto/user/' + ctx.params.uid, { page: ctx.page });
  ctx.title(d.owner.nick + ' — фотоальбомы');
  const out = [crumbs({ href: '/id' + d.owner.id, title: d.owner.nick }, { title: 'Фотоальбомы' }), ui.pageHead('Альбомы: ' + d.owner.nick)];
  if (d.mine) out.push(h('details.card.pad', { open: ctx.query.create === '1' }, h('summary', icon('plus', 16), ' Создать альбом'), albumForm({}, async v => { const r = await api.post('/api/foto/album', v); ui.ok('Фотоальбом создан'); router.go(`/foto/${r.owner}/${r.id}`); }, 'Создать')));
  out.push(d.items.length ? h('div.album-grid', d.items.map(al => albumTile({ ...al, owner: null }))) : ui.card(ui.empty('Нет альбомов', 'camera')), ui.pager(d));
  return out;
});

// ───────── альбом
router.add('/foto/:uid/:g', async ctx => {
  const { uid, g } = ctx.params;
  const reload = () => router.render();
  const d = await api.get('/api/foto/album/' + g, { page: ctx.page });
  ctx.title(d.name);
  const head = [crumbs({ href: '/id' + d.owner.id, title: d.owner.nick }, { href: '/foto/user/' + d.owner.id, title: 'Альбомы' }, { title: d.name }), ui.pageHead(d.name, h('div.row', d.can_edit ? ui.btn('', { ic: 'trash', kind: 'danger', small: true, title: 'Удалить альбом', onclick: async () => { if (await ui.confirm(`Удалить альбом «${d.name}» и все фотографии в нём?`, { danger: true })) { await api.del('/api/foto/album/' + g); ui.ok('Альбом удалён'); router.go('/foto/user/' + d.owner.id); } } }) : null)),
    d.opis ? h('div.muted', { html: d.opis }) : null];
  if (d.blocked) {
    if (d.blocked === 'password') {
      const p = h('input.input', { type: 'password', placeholder: 'Пароль' });
      const go = async () => { await api.post(`/api/foto/album/${g}/unlock`, { password: p.value }); reload(); };
      p.onkeydown = e => e.key === 'Enter' && go();
      return [...head, h('div.card.pad.stack', h('div', icon('lock', 16), ' Альбом защищён паролем'), p, ui.btn('Войти', { kind: 'primary', onclick: go }))];
    }
    return [...head, h('div.card.pad.muted', icon('lock', 16), d.blocked === 'friends' ? ' Просматривать альбом могут только друзья.' : ' Пользователь запретил просмотр этого альбома.')];
  }
  const out = [...head];
  if (d.mine) out.push(uploadBox(d, reload));
  out.push(d.items.length ? h('div.photo-grid', d.items.map(f => h('a.photo-tile', { href: `/foto/${d.owner.id}/${d.id}/${f.id}` },
    h('img', { src: f.src, alt: f.name, loading: 'lazy', class: f.metka ? 'blur' : '' }), f.metka ? h('span.badge.danger.tl', '18+') : null, f.avatar ? h('span.badge.ok.tl', 'аватар') : null,
    h('div.photo-cap', h('span.clip', f.name), h('span.small', icon('message', 11), ' ', f.komm, ' · ★ ', f.rating))))) : ui.card(ui.empty('Фотографий нет', 'camera')), ui.pager(d));
  if (d.can_edit) out.push(h('details.card.pad', h('summary', icon('settings', 16), ' Настройки альбома'), albumForm(d, async v => { await api.put('/api/foto/album/' + g, v); ui.ok('Сохранено'); reload(); }, 'Сохранить')));
  return out;
});

function uploadBox(d, reload) {
  const name = h('input.input', { maxlength: 32, placeholder: 'Название (по умолчанию — имя файла)' });
  const file = h('input', { type: 'file', accept: 'image/jpeg,image/png,image/gif', multiple: false });
  const opis = h('textarea.input', { rows: 2, maxlength: 1024, placeholder: 'Описание' });
  const metka = h('input', { type: 'checkbox' }), ava = h('input', { type: 'checkbox' });
  const send = async () => {
    if (!file.files[0]) return ui.toast('Выберите файл', 'warn');
    const r = await api.upload(`/api/foto/album/${d.id}/upload`, { name: name.value, file: file.files[0], opis: opis.value, metka: metka.checked ? '1' : '0', avatar: ava.checked ? '1' : '0' });
    ui.ok('Фотография загружена'); router.go(`/foto/${d.owner.id}/${d.id}/${r.id}`);
  };
  return h('details.card.pad', h('summary', icon('camera', 16), ' Загрузить фотографию'), h('div.stack',
    h('div.field', h('label', 'Файл'), file), h('div.field', h('label', 'Название'), name), h('div.field', h('label', 'Описание'), opis),
    h('label.check', metka, ' Метка ', h('b.danger-text', '18+')), h('label.check', ava, ' Сделать аватаром'), ui.btn('Загрузить', { ic: 'upload', kind: 'primary', onclick: send }),
    h('div.small.muted', 'Фото не должны нарушать закон, права третьих лиц, быть непристойными, рекламными или порнографическими.')));
}

// ───────── фотография
router.add('/foto/:uid/:g/:f', async ctx => {
  const { uid, g, f } = ctx.params;
  const reload = () => router.render();
  const d = await api.get('/api/foto/photo/' + f, { page: ctx.query.page || ctx.page, sort: ctx.query.sort });
  ctx.title(d.name);
  const head = [crumbs({ href: '/id' + d.owner.id, title: d.owner.nick }, { href: '/foto/user/' + d.owner.id, title: 'Альбомы' }, { href: `/foto/${d.owner.id}/${d.gallery.id}`, title: d.gallery.name }, { title: d.name }), ui.pageHead(d.name, d.metka ? h('span.badge.danger', '18+') : null)];
  if (d.blocked) return [...head, h('div.card.pad.muted', icon('lock', 16), d.blocked === 'password' ? ' Альбом защищён паролем.' : d.blocked === 'friends' ? ' Только для друзей.' : ' Просмотр запрещён.', ' ', link(`/foto/${d.owner.id}/${d.gallery.id}`, 'К альбому'))];
  if (d.adult) return [...head, d.need_login ? h('div.card.pad', 'Фото 18+ доступно только зарегистрированным пользователям. ', link('/login', 'Войти'))
    : h('div.card.pad.stack', h('div', icon('alert', 16), ' Фото содержит изображения эротического характера. Если вам 18+ — можете продолжить.'), ui.btn('Продолжить просмотр', { kind: 'primary', onclick: async () => { await api.post('/api/foto/abuld'); reload(); } }))];

  const out = [...head];
  const photoWrap = h('div.card.pad.photo-view', { style: { position: 'relative' } },
    h('a', { href: d.src_orig + '?dl=1', title: 'Скачать оригинал (двойной тап — оценить)' }, h('img', { src: d.src, alt: d.name })),
    d.plus5 ? h('div.badge.ok', '5+ ×' + d.plus5) : null);
  let lastFotoTap = 0;
  photoWrap.addEventListener('click', async e => {
    const now = Date.now();
    if (now - lastFotoTap < 300) {
      e.preventDefault();
      lastFotoTap = 0;
      const rect = photoWrap.getBoundingClientRect();
      const heart = h('div.dtap-heart', { style: { left: (e.clientX - rect.left) + 'px', top: (e.clientY - rect.top) + 'px' } }, icon('heart', 72));
      photoWrap.append(heart);
      setTimeout(() => heart.remove(), 750);
      if (me() && d.can_rate && !d.my_rating) {
        await api.post(`/api/foto/photo/${f}/rating`, { rating: me().vip ? 6 : 5 });
        ui.ok(me().vip ? 'Оценка 5+ принята!' : 'Оценка 5 принята!');
        reload();
      }
      return;
    }
    lastFotoTap = now;
  });
  out.push(photoWrap);
  // оценка
  if (d.can_rate !== undefined && me()) {
    const rate = async n => { await api.post(`/api/foto/photo/${f}/rating`, { rating: n }); ui.ok('Ваша оценка принята'); reload(); };
    out.push(h('div.card.pad.row', d.my_rating ? h('span', 'Ваша оценка: ', h('b', d.my_rating === 6 ? '5+' : d.my_rating)) : d.can_rate ? [h('span.small.muted', 'Оценить:'), ...[6, 5, 4, 3, 2, 1].map(n => ui.btn(n === 6 ? '5+' : String(n), { small: true, kind: n === 6 ? 'primary' : undefined, onclick: () => rate(n) }))] : h('span.muted.small', 'Свои фото оценивать нельзя'),
      h('span.grow'), h('span.small.muted', `Рейтинг: ${d.rating} (голосов: ${d.votes}${d.avg ? ', средняя ' + d.avg.toFixed(1) : ''})`)));
  } else out.push(h('div.card.pad.small.muted', `Рейтинг: ${d.rating} (голосов: ${d.votes})`));
  const acts = [];
  if (me()) acts.push(ui.btn(d.bookmarked ? 'Из закладок' : 'В закладки', { ic: 'star', small: true, kind: d.bookmarked ? 'primary' : undefined, onclick: async () => { await api.post('/api/bookmark/toggle', { type: 'foto', id: +f }); reload(); } }), h('span.badge', d.bookmarks));
  if (d.can_edit && d.owner.id === me()?.id && !d.avatar) acts.push(ui.btn('Сделать аватаром', { ic: 'user', small: true, onclick: async () => { await api.post(`/api/foto/photo/${f}/avatar`); ui.ok('Фотография установлена на главной'); reload(); } }));
  if (d.can_edit) acts.push(ui.btn('Удалить', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить фотографию?', { danger: true })) { await api.del('/api/foto/photo/' + f); ui.ok('Фотография удалена'); router.go(`/foto/${d.owner.id}/${d.gallery.id}`); } } }));
  out.push(h('div.row', ...acts, h('span.grow'), d.newer ? link(`/foto/${d.owner.id}/${d.gallery.id}/${d.newer}`, '← Новее') : null, d.older ? link(`/foto/${d.owner.id}/${d.gallery.id}/${d.older}`, 'Старее →') : null));
  if (d.opis) out.push(h('div.card.pad', { html: d.opis }));
  if (d.can_edit) {
    const name = h('input.input', { value: d.name, maxlength: 32 }), opis = h('textarea.input', { rows: 3, maxlength: 1024 }, d.raw_opis || ''), metka = h('input', { type: 'checkbox', checked: d.metka });
    out.push(h('details.card.pad', h('summary', icon('edit', 16), ' Редактировать'), h('div.stack', h('div.field', h('label', 'Название'), name), h('div.field', h('label', 'Описание'), opis), h('label.check', metka, ' Метка 18+'),
      ui.btn('Сохранить', { ic: 'check', kind: 'primary', onclick: async () => { await api.put('/api/foto/photo/' + f, { name: name.value, opis: opis.value, metka: metka.checked }); ui.ok('Сохранено'); reload(); } }))));
  }
  // комментарии
  const k = d.komms;
  const ed = ui.bbEditor({ rows: 3, placeholder: 'Комментарий', maxlength: 1024 });
  let replyTo = 0;
  const answer = p => { replyTo = p.user.id; ed.ta.value = `[b]${p.user.nick}[/b], `; ed.ta.focus(); };
  out.push(h('h3', 'Комментарии (' + k.total + ')'));
  if (me() && k.total) out.push(h('div.chips', ['Вверху', 'Внизу'].map((l, v) => h('a.chip' + (d.sort === v ? '.active' : ''), { href: `/foto/${uid}/${g}/${f}?sort=${v}` }, l))));
  out.push(ui.card(k.items.length ? h('div.posts', k.items.map(p => ui.post({ user: p.user, time: p.time, body: h('div.post-body', { html: p.msg }), id: 'k' + p.id, actions: [
    p.can_reply && d.can_komm ? ui.btn('', { ic: 'corner-up-left', small: true, title: 'Ответить', onclick: () => answer(p) }) : null,
    p.can_delete ? ui.btn('', { ic: 'trash', small: true, onclick: async () => { if (await ui.confirm('Удалить комментарий?', { danger: true })) { await api.del('/api/foto-komm/' + p.id); reload(); } } }) : null].filter(Boolean) }))) : ui.empty('Нет комментариев', 'message')), ui.pager(k));
  out.push(d.can_komm ? h('div.card.pad.stack', ed.el, h('div.row', h('span.grow'), ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: async () => { await api.post(`/api/foto/photo/${f}/komm`, { msg: ed.value, reply_to: replyTo }); ui.ok('Сообщение добавлено'); reload(); } })))
    : me() ? h('div.card.pad.muted', 'Комментировать эту фотографию вам нельзя.') : h('div.card.pad.muted', link('/login', 'Войдите'), ', чтобы комментировать.'));
  return out;
});

// ───────── мой аватар
router.add('/avatar', async ctx => {
  ctx.title('Мой аватар');
  const u = me();
  const reload = async () => { const { refreshMe } = await import('../core.js'); await refreshMe(); router.render(); };
  const file = h('input', { type: 'file', accept: 'image/jpeg,image/png,image/gif' });
  const count = await api.get('/api/foto/user/' + u.id).then(r => r.photos).catch(() => 0);
  return [ui.pageHead('Мой аватар'), h('div.card.pad.stack.center', ui.avatar(u, 128), h('div.small.muted', 'Аватар отображается на вашей странице и рядом с сообщениями'),
    h('div.field', file), h('div.row', ui.btn('Загрузить аватар', { ic: 'upload', kind: 'primary', onclick: async () => { if (!file.files[0]) return ui.toast('Выберите файл', 'warn'); await api.upload('/api/foto/avatar', { file: file.files[0] }); ui.ok('Аватар обновлён'); reload(); } }),
      u.avatar ? ui.btn('Удалить аватар', { ic: 'trash', kind: 'danger', onclick: async () => { await api.del('/api/foto/avatar'); ui.ok('Аватар удалён'); reload(); } }) : null)),
    h('div.card.pad', 'Или выберите любое фото в своём фотоальбоме и нажмите «Сделать аватаром». ', link('/foto/user/' + u.id, `Мои фотографии (${count})`))];
}, { auth: true });

// ───────── админка
router.add('/admin/foto', async ctx => {
  ctx.title('Настройки фотогалереи');
  const s = await api.get('/api/admin/foto/settings');
  return [ui.pageHead('Настройки фотогалереи'), ui.card(h('div.pad', ui.form([
    { name: 'max_upload_foto_x', label: 'Максимальная ширина загружаемого фото (px)', type: 'number', value: s.max_upload_foto_x },
    { name: 'max_upload_foto_y', label: 'Максимальная высота загружаемого фото (px)', type: 'number', value: s.max_upload_foto_y }],
    { submit: 'Сохранить', onSubmit: async v => { await api.put('/api/admin/foto/settings', { max_upload_foto_x: +v.max_upload_foto_x, max_upload_foto_y: +v.max_upload_foto_y }); ui.ok('Настройки сохранены'); } })))];
}, { auth: true, access: 'adm_set_foto' });
