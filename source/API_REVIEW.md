# Возможности API Яндекс Музыки для YaGo

Исходный обзор до 0.8.0: проверка документации и `internal/music/api.go`: 16 ноября 2026 года.

Основа — неофициальная библиотека [MarshalX/yandex-music-api](https://github.com/MarshalX/yandex-music-api) и её документация. Это результат обратной разработки, а не договор на стабильность API со стороны Яндекса. Ниже наличие метода в документации отделено от готовой функции YaGo. Запросы, изменяющие аккаунт, при исследовании не выполнялись.

В YaGo уже есть вход по коду, статус аккаунта, поиск треков (первая страница), получение треков, избранное, чтение плейлистов, добавление лайка, сессии «Моей волны» с обратной связью и получение потока с выбором Basic/HQ. Оффлайн-плейлисты и восстановление позиции реализованы локально; это не облачная синхронизация Яндекса.

| Возможность | Методы / источник | Что добавить в YaGo |
| --- | --- | --- |
| Снять лайк, «Не рекомендовать» | [likes](https://ym.marshal.dev/yandex_music._client.likes/): `users_likes_tracks_remove`, `users_dislikes_tracks_add/remove` | Полноценные переключатели лайка и запрета рекомендаций |
| Облачные плейлисты | [playlists](https://ym.marshal.dev/yandex_music._client.playlists/): `users_playlists_create/delete/name/change`, `users_playlists_insert_track/delete_track` | Создание, переименование, добавление, удаление и перестановка треков в аккаунте; учитывать revision при изменении |
| Рекомендации к плейлисту | `users_playlists_recommendations` | «Дополнить похожими» с выбором перед добавлением |
| Текст песни | [tracks](https://ym.marshal.dev/yandex_music._client.tracks/): `tracks_lyrics` | Обычный текст и LRC с подсветкой строк; текст может отсутствовать |
| Похожие песни | `tracks_similar` | «Похожие на этот трек» |
| Полный поиск | [search](https://ym.marshal.dev/yandex_music._client.search/): `search`, `search_suggest` | Страницы результатов, подсказки, альбомы, исполнители, плейлисты; подкасты и эпизоды запрашивать отдельными типами |
| Альбомы и исполнители | [albums](https://ym.marshal.dev/yandex_music._client.albums/), [artists](https://ym.marshal.dev/yandex_music._client.artists/) | Карточки, дискография, список песен и похожие исполнители |
| Настройки волны | [wave](https://ym.marshal.dev/yandex_music._client.wave/): `rotor_wave_settings`, `rotor_wave_last`; [radio](https://ym.marshal.dev/yandex_music._client.radio/) | Получать доступные занятия и настройки с сервера; станции по треку/исполнителю; не зашивать неподтверждённые значения |
| История аккаунта | [music_history](https://ym.marshal.dev/yandex_music._client.music_history/): `music_history`, `music_history_items` | «Недавно слушали» с подгрузкой подробностей по необходимости |
| Чарты и подборки | [landing](https://ym.marshal.dev/yandex_music._client.landing/): `chart`, `landing`, `feed`, `new_releases`, `new_playlists` | Чарт, новинки, персональные подборки |
| Между устройствами | [Ynison](https://ym.marshal.dev/yandex_music.ynison.simple/) | Состояние, устройства, очередь и удалённое управление; отдельная интеграция WebSocket/protobuf, разрешение конфликтов устройств |
| Lossless | `tracks_file_info(quality='lossless')` | Дополнительные кодеки/декодер и способ доставки `encraw`; нужна доступность по подписке и треку. Текущий MP3-плеер автоматически FLAC не поддерживает |

Рекомендуемый порядок: снять лайк/дизлайк → облачные плейлисты → поиск с подгрузкой и карточки альбомов/исполнителей → текст песни → настройки волны и история → Ynison → lossless.

Для экономии памяти: запрашивать страницы по требованию, ограничивать кэш метаданных и обложек, LRC загружать только при открытии, Ynison подключать по необходимости. Переносить нужные HTTP-методы в Go; подключать Python для этого не требуется.

В 0.8.0 реализованы снятие лайка, дизлайк трека/исполнителя с локальной защитой, поиск с типами и страницами, подсказки по нажатию, недавняя история, настройки волны и гостевые фрагменты. Подробности — DISCOVERY.md; память — DISCOVERY_MEMORY.md. Тексты песен исключены по решению пользователя.

## Проверка VK, Звука и КИОН: исходники и живые запросы

Проверены открытые реализации и выполнены реальные анонимные HTTP-запросы.
Токены VK_MUSIC_TOKEN, ZVUK_TOKEN и KION_TOKEN в тестовом окружении не настроены.
Авторизованные библиотеки, полные треки и их перемотка не проверены. Наличие
кода в стороннем проекте не принимается за подтверждение работающей интеграции.

| Сервис | Наблюдаемый результат | Ограничение вывода |
| --- | --- | --- |
| VK | audio.get, audio.search, catalog.getAudio: HTTP 200 с error_code=15 | Сервер отказал в доступе. Полноценная музыкальная сессия отсутствует; не доказывает поломку методов для авторизованного клиента. |
| Звук | api/tiny/profile и POST api/v1/graphql с quickSearch: HTTP 418, HTML вместо JSON | Не получен даже анонимный токен/каталог; защита блокирует проверку из этого окружения. Первая попытка fetch также завершалась транспортной ошибкой. |
| КИОН/МТС | music.mts.ru/ya_proxy_api: account/status, search, download-info и rotor вернули HTTP 404; ya_api/account/status тоже 404 | music.kion.ru перенаправляет на music.mts.ru и страницу капчи. Это не подтверждение недоступности всего сервиса и не подтверждение актуальности путей для авторизованной сессии. |

Ни один аудиопоток в этих проверках не получен. Поэтому полный трек, фактический
кодек/битрейт, HTTP Range, работа декодера, оффлайн и рекомендации пока не приняты
как проверенные. Изменений в аккаунтах не выполнялось; капчи не решались.

Дополнительные выводы по исходникам:

- VK: [MusiMuX API](https://github.com/MuXolotl/MusiMuX/blob/main/docs/API.md)
  описывает отдельную авторизацию музыкального клиента, каталог, миксы и
  курсорную пагинацию. Публичный токен обычного приложения не равнозначен
  музыкальной сессии. Не проверено на аккаунте.
- Звук: [client.py](https://github.com/trudenboy/zvuk-music/blob/main/zvuk_music/client.py)
  и [request.py](https://github.com/trudenboy/zvuk-music/blob/main/zvuk_music/utils/request.py)
  содержат GraphQL и Tiny API, прямые потоки и отдельные DRM-варианты. Найдены
  add_to_hidden/remove_from_hidden и тип ARTIST: есть реализация серверного
  скрытия треков и исполнителей. Это уточняет прежнюю оценку «не подтверждено
  в исходниках», но серверная работа ещё не проверена. synthesisPlaylistBuild
  объединяет двух авторов; это не доказательство аналога бесконечной «Моей волны».
- КИОН: [api_client.py](https://github.com/trudenboy/ma-provider-kion-music/blob/main/provider/api_client.py)
  использует yandex_music.ClientAsync с другим base_url, rotor и feedback.
  [streaming.py](https://github.com/trudenboy/ma-provider-kion-music/blob/main/provider/streaming.py)
  выбирает MP3/AAC/FLAC, включая MP4-контейнеры. Наш нынешний MP3-декодер
  автоматически не поддерживает все эти форматы.

Замеры: `.tools/service-audit/live-results.json`, `network-results.json`,
`graphql-live-result.json`. Там нет токенов аккаунтов и подписанных ссылок на
музыку; query-параметры страниц капчи удалены из итогового network-results.json.
Код YaGo и публичные релизы по результатам этого исследования не менялись.
