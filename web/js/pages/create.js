import { h, api, ui, router, state, icon, can } from '../core.js';

const me = () => state.user;

function openStatusModal() {
  const inp = h('textarea.input', { rows: 3, maxlength: 512, placeholder: 'Что у вас нового? Расскажите друзьям…' });
  const modal = ui.modal({
    title: 'Новый статус',
    body: h('div.stack', inp),
    actions: [
      { label: 'Отмена' },
      {
        label: 'Опубликовать', kind: 'primary',
        onclick: async () => {
          if (!inp.value.trim()) { ui.toast('Введите текст статуса', 'warn'); return false; }
          await api.post('/api/status', { msg: inp.value.trim() });
          modal.close();
          ui.ok('Статус обновлён');
          router.go('/id' + me().id);
          return false;
        },
      },
    ],
  });
  setTimeout(() => inp.focus(), 50);
}

function openWallModal() {
  const ed = ui.bbEditor({ rows: 4, placeholder: 'Напишите запись на своей стене (поддерживаются BB-коды и #хештеги)…', maxlength: 1024 });
  const modal = ui.modal({
    title: 'Новая запись на стене',
    body: h('div.stack', ed.el),
    actions: [
      { label: 'Отмена' },
      {
        label: 'Опубликовать', kind: 'primary',
        onclick: async () => {
          if (!ed.value.trim()) { ui.toast('Введите текст записи', 'warn'); return false; }
          await api.post(`/api/user/${me().id}/stena`, { msg: ed.value.trim() });
          modal.close();
          ui.ok('Запись опубликована на стене');
          router.go('/id' + me().id);
          return false;
        },
      },
    ],
  });
}

function openCreateGroupModal() {
  const nameIn = h('input.input', { placeholder: 'Название сообщества', maxlength: 64 });
  const opisIn = h('textarea.input', { rows: 3, placeholder: 'Краткое описание (необязательно)', maxlength: 500 });
  const privCb = h('input', { type: 'checkbox' });
  const modal = ui.modal({
    title: 'Создать сообщество',
    body: h('div.stack',
      h('div.field', h('label', 'Название'), nameIn),
      h('div.field', h('label', 'Описание'), opisIn),
      h('label.check', privCb, ' Приватное сообщество (вступление по заявке)')),
    actions: [
      { label: 'Отмена' },
      {
        label: 'Создать', kind: 'primary',
        onclick: async () => {
          if (!nameIn.value.trim()) { ui.toast('Укажите название сообщества', 'warn'); return false; }
          const r = await api.post('/api/groups', { name: nameIn.value.trim(), opis: opisIn.value.trim(), private: privCb.checked });
          modal.close();
          ui.ok('Сообщество создано');
          router.go('/groups/' + r.id);
          return false;
        },
      },
    ],
  });
  setTimeout(() => nameIn.focus(), 50);
}

// ───────── Центр быстрой публикации (/create)
router.add('/create', async ctx => {
  ctx.title('Создать публикацию');
  const u = me();

  const card = ({ title, desc, ic, color, badge, onclick, href }) => {
    const tag = href ? 'a.create-card' : 'button.create-card';
    const attrs = href ? { href } : { type: 'button', onclick };
    return h(tag, attrs,
      h('div.create-ico', { style: { background: color } }, icon(ic, 24)),
      h('div.create-body',
        h('div.create-title', title, badge ? h('span.badge', { style: { marginLeft: '6px' } }, badge) : null),
        h('div.create-desc', desc)),
      icon('chevron-right', 18));
  };

  const quick = [
    card({
      title: 'История (24 часа)',
      desc: 'Фото или текстовая карточка на градиенте — исчезнет через сутки',
      ic: 'zap', color: 'linear-gradient(135deg,#ec4899,#f59e0b)', badge: 'Быстро',
      onclick: async () => {
        const { openCreateStoryModal } = await import('./stories.js');
        openCreateStoryModal(() => router.go('/'));
      },
    }),
    card({
      title: 'Загрузить клип',
      desc: 'Короткое вертикальное видео в ленту Shorts',
      ic: 'video', color: 'linear-gradient(135deg,#6366f1,#8b5cf6)',
      onclick: async () => {
        const { openUploadModal } = await import('./shorts.js');
        openUploadModal(60, 100, () => router.go('/shorts/user/' + u.id));
      },
    }),
    card({
      title: 'Запись на стену',
      desc: 'Поделиться мыслями, BB-кодами и #хештегами у себя в профиле',
      ic: 'edit', color: 'linear-gradient(135deg,#0ea5e9,#2563eb)',
      onclick: openWallModal,
    }),
    card({
      title: 'Обновить статус',
      desc: 'Короткая заметка в шапке вашей страницы',
      ic: 'message-circle', color: 'linear-gradient(135deg,#10b981,#14b8a6)',
      onclick: openStatusModal,
    }),
  ];

  const more = [
    card({
      title: 'Запись в блог',
      desc: 'Полноценная статья в дневнике с оформлением',
      ic: 'book', color: '#8b5cf6',
      href: '/notes/add',
    }),
    card({
      title: 'Создать сообщество',
      desc: 'Своя группа с блогом, чатом, опросами, форумом и ботами',
      ic: 'users', color: '#f97316',
      onclick: openCreateGroupModal,
    }),
    card({
      title: 'Загрузить фото',
      desc: 'Добавить снимки в личные фотоальбомы',
      ic: 'camera', color: '#ec4899',
      href: '/foto/user/' + u.id,
    }),
    card({
      title: 'Загрузить файл',
      desc: 'Добавить файл в личное хранилище или обменник',
      ic: 'upload', color: '#0ea5e9',
      href: `/user/personalfiles/${u.id}/0/upload`,
    }),
    card({
      title: 'Тема на форуме',
      desc: 'Открыть новое обсуждение в разделах форума',
      ic: 'forum', color: '#3b82f6',
      href: '/forum',
    }),
    can('adm_news') ? card({
      title: 'Новость сайта',
      desc: 'Официальная публикация на главной странице',
      ic: 'rss', color: '#ef4444', badge: 'Админ',
      href: '/news/add',
    }) : null,
  ].filter(Boolean);

  return [
    ui.pageHead('Создать публикацию'),
    h('div.small.muted', 'Выберите, чем хотите поделиться — всё собрано в одном месте.'),
    h('div.create-grid', ...quick),
    h('div.create-sub', 'Другие разделы'),
    h('div.create-grid', ...more),
  ];
}, { auth: true });
