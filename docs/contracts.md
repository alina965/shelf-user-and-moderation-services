# User Service: эндпоинты

Локальный адрес: `http://localhost:8080`. Пути ниже указаны без `/api/v1`: этот внешний префикс настраивается на Gateway. UUID, токены и секреты в примерах нужно заменить своими. Примеры ответов иллюстративные; значения зависят от данных и настроек сервиса.

Машиночитаемая спецификация: [openapi.yaml](openapi.yaml).

## Общие правила

- JSON-запросы: `Content-Type: application/json`. Если тело не показано, оно не требуется. Ответ `204` — без тела.
- **Ошибки сессии** для пользовательской авторизации: `401 UNAUTHORIZED` — токен отсутствует, неверен или просрочен; `401 SESSION_REVOKED` — сессия отозвана/отсутствует; `401 SESSION_EXPIRED` — срок сессии истёк; `503 SERVICE_UNAVAILABLE` — проверка недоступна.
- **Служебные ошибки** для `/internal/*`: `401 SERVICE_UNAUTHORIZED` — неверный/отсутствующий служебный секрет; `503 SERVICE_UNAVAILABLE` — внутренний API не настроен или обязательная проверка недоступна. Секреты берутся из `INTERNAL_SERVICE_TOKENS`, в мобильное приложение не передаются.
- Для прикладных обработчиков также возможна `500 INTERNAL_ERROR` при непредвиденном сбое. Неизвестные маршруты и аварийные ответы Gin могут иметь другой формат.
- В списках `limit` — от 1 до 20, по умолчанию 20. Для следующей страницы передавай `cursor` из `next_cursor`; на последней странице он `null`. Пустой список: `{"items":[],"next_cursor":null}`.
- Каждый ответ содержит `X-Request-ID`. Прикладные ошибки имеют общий формат:

```json
{"error":{"code":"FORBIDDEN","message":"Недостаточно прав"},"request_id":"req_example"}
```

Ошибки регистрации и входа могут содержать `error.fields` с сообщениями по полям. Конфликты заявок — `error.details.friend_request_id`.

## POST /auth/register

Регистрация без авторизации.

```http
POST /auth/register
Content-Type: application/json

{"email":"anna@example.com","username":"Anna","password":"ExamplePass123!","password_confirmation":"ExamplePass123!"}
```

Успех: `201`. Сессия создаётся при входе.

Пример тела ответа:

```json
{
  "user_id": "11111111-1111-4111-8111-111111111111"
}
```

Ошибки: `400 INVALID_JSON`, `400 INVALID_REGISTRATION` — некорректные поля; `409 ACCOUNT_EXISTS` — email или username занят. При сочетании ошибок формата и уникальности возвращается `400` с ошибками всех полей.

## POST /auth/login

Вход по email или username.

```http
POST /auth/login
Content-Type: application/json

{"identifier":"anna@example.com","password":"ExamplePass123!"}
```

Успех: `200`, `access_token`, `refresh_token`, `expires_in` (секунды), `user` — собственный профиль. Каждый вход создаёт отдельную сессию.

Пример тела ответа:

```json
{
  "access_token": "<access_token>",
  "refresh_token": "<refresh_token>",
  "expires_in": 899,
  "user": {
    "id": "11111111-1111-4111-8111-111111111111",
    "email": "anna@example.com",
    "username": "Anna",
    "avatar_url": null,
    "banner_url": null,
    "description": null,
    "friends_count": 0,
    "privacy": {
      "profile_visibility": "public",
      "description_visibility": "public",
      "friends_visibility": "public",
      "communities_visibility": "public"
    }
  }
}
```

Ошибки: `400 INVALID_JSON`, `400 INVALID_LOGIN`; `401 BAD_CREDENTIALS` — неверные данные; `429 LOGIN_LOCKED` — вход временно заблокирован; `503 SERVICE_UNAVAILABLE` — обязательная проверка недоступна; `404 PROFILE_NOT_FOUND` — профиль исчез во время входа.

## POST /auth/refresh

Обновление пары токенов без Bearer-авторизации.

```http
POST /auth/refresh
Content-Type: application/json

{"refresh_token":"<refresh_token>"}
```

Успех: `200`, новые `access_token`, `refresh_token`, `expires_in`. Старый refresh больше не работает; исходный срок сессии не продлевается.

Пример тела ответа:

```json
{
  "access_token": "<new_access_token>",
  "refresh_token": "<new_refresh_token>",
  "expires_in": 899
}
```

Ошибки: `400 INVALID_JSON`; `401 INVALID_REFRESH_TOKEN`, `401 SESSION_REVOKED`, `401 SESSION_EXPIRED`; `503 SERVICE_UNAVAILABLE`.

## POST /auth/logout

Отзыв только текущей сессии.

```http
POST /auth/logout
Authorization: Bearer <access_token>
```

Успех: `204`. Другие сессии остаются активными.

Тело ответа отсутствует.

Ошибки: ошибки сессии. Повторный выход с отозванным access возвращает `401 SESSION_REVOKED`.

## GET /users/me

Собственный профиль с email, описанием, числом друзей и privacy. `avatar_url` и `banner_url` пока `null`.

```http
GET /users/me
Authorization: Bearer <access_token>
```

Успех: `200`, поля `id`, `email`, `username`, `avatar_url`, `banner_url`, `description`, `friends_count`, `privacy`.

Пример тела ответа:

```json
{
  "id": "11111111-1111-4111-8111-111111111111",
  "email": "anna@example.com",
  "username": "Anna",
  "avatar_url": null,
  "banner_url": null,
  "description": null,
  "friends_count": 0,
  "privacy": {
    "profile_visibility": "public",
    "description_visibility": "public",
    "friends_visibility": "public",
    "communities_visibility": "public"
  }
}
```

Ошибки: ошибки сессии; `404 PROFILE_NOT_FOUND`.

## GET /users

Публичный поиск по username; `q` обязателен, минимум 2 символа после удаления крайних пробелов. Переданный Bearer здесь не проверяется.

```http
GET /users?q=anna&limit=20
```

Успех: `200`, `items` с публичными карточками (`id`, `username`, `avatar_url`) и `next_cursor`.

Пример тела ответа:

```json
{
  "items": [
    {
      "id": "11111111-1111-4111-8111-111111111111",
      "username": "Anna",
      "avatar_url": null
    }
  ],
  "next_cursor": null
}
```

Ошибки: `400 QUERY_REQUIRED`, `400 QUERY_TOO_SHORT`, `400 INVALID_LIMIT`, `400 INVALID_CURSOR`.

## GET /users/{id}

Профиль с учётом приватности, дружбы и блокировок. Для гостя убери Authorization; переданный неверный токен возвращает ошибку.

```http
GET /users/11111111-1111-4111-8111-111111111111
Authorization: Bearer <access_token>
```

Успех: `200`, `id`, `username`, `avatar_url`, `banner_url`, `is_private`; `description` и `friends_count` выдаются при наличии доступа. Email отсутствует. Закрытый профиль тоже возвращает `200`, с `is_private:true` и без закрытых полей.

Пример тела ответа:

```json
{
  "id": "11111111-1111-4111-8111-111111111111",
  "username": "Anna",
  "avatar_url": null,
  "banner_url": null,
  "is_private": false,
  "description": "Люблю книги и кино",
  "friends_count": 0
}
```

Если профиль закрыт для зрителя (тоже `200`):

```json
{
  "id": "11111111-1111-4111-8111-111111111111",
  "username": "Anna",
  "avatar_url": null,
  "banner_url": null,
  "is_private": true
}
```

Ошибки: `404 PROFILE_NOT_FOUND` (включая некорректный/нулевой UUID), `503 SERVICE_UNAVAILABLE`; при переданном токене — ошибки сессии.

## POST /users/{id}/friend-requests

Отправка заявки. Тело должно отсутствовать. Ключ: 1–128 символов `A–Z`, `a–z`, `0–9`, `.`, `_`, `:`, `-`.

```http
POST /users/22222222-2222-4222-8222-222222222222/friend-requests
Authorization: Bearer <access_token>
Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000
```

Успех: `201`, `id`, `status:"pending"`, `created_at`. Дружба появится после принятия. Повтор с тем же ключом/получателем в течение 24 часов возвращает сохранённый результат, даже если заявка уже закрыта; для нового действия нужен новый ключ.

Пример тела ответа:

```json
{
  "id": "33333333-3333-4333-8333-333333333333",
  "status": "pending",
  "created_at": "2026-11-26T10:00:00Z"
}
```

Ошибки: ошибки сессии; `400 IDEMPOTENCY_KEY_REQUIRED`, `400 INVALID_IDEMPOTENCY_KEY`, `400 INVALID_REQUEST` (передано тело), `400 SELF_ACTION`; `403 INTERACTION_BLOCKED`; `404 PROFILE_NOT_FOUND`; `409 IDEMPOTENCY_CONFLICT` (ключ использован для другого получателя), `409 ALREADY_FRIENDS`, `409 REQUEST_ALREADY_EXISTS`, `409 INCOMING_REQUEST_EXISTS`; `503 SERVICE_UNAVAILABLE`.

## GET /friend-requests

Собственные ожидающие заявки. Обязательный `direction`: `incoming` или `outgoing`.

```http
GET /friend-requests?direction=incoming&limit=20
Authorization: Bearer <access_token>
```

Успех: `200`, `items` с `id`, `status`, `created_at`, `user` (карточка другой стороны), и `next_cursor`.

Пример тела ответа:

```json
{
  "items": [
    {
      "id": "33333333-3333-4333-8333-333333333333",
      "status": "pending",
      "created_at": "2026-11-26T10:00:00Z",
      "user": {
        "id": "22222222-2222-4222-8222-222222222222",
        "username": "anna_books",
        "avatar_url": null
      }
    }
  ],
  "next_cursor": null
}
```

Ошибки: ошибки сессии; `400 INVALID_DIRECTION`, `400 INVALID_LIMIT`, `400 INVALID_CURSOR`; `503 SERVICE_UNAVAILABLE`.

## POST /friend-requests/{id}/accept

Принятие заявки её получателем.

```http
POST /friend-requests/33333333-3333-4333-8333-333333333333/accept
Authorization: Bearer <recipient_access_token>
```

Успех: `200 {"status":"accepted"}`. Повтор успешен, пока дружба существует.

Пример тела ответа:

```json
{
  "status": "accepted"
}
```

Ошибки: ошибки сессии; `403 FORBIDDEN` (принимает отправитель), `403 INTERACTION_BLOCKED`; `404 FRIEND_REQUEST_NOT_FOUND`, `404 PROFILE_NOT_FOUND`; `409 FRIEND_REQUEST_NOT_PENDING`; `503 SERVICE_UNAVAILABLE`.

## DELETE /friend-requests/{id}

Отправитель отменяет заявку, получатель отклоняет её.

```http
DELETE /friend-requests/33333333-3333-4333-8333-333333333333
Authorization: Bearer <access_token>
```

Успех: `204`, включая повтор своего отказа/отмены. Принятую заявку этим методом удалить нельзя.

Тело ответа отсутствует.

Ошибки: ошибки сессии; `404 FRIEND_REQUEST_NOT_FOUND`, `404 PROFILE_NOT_FOUND`; `409 FRIEND_REQUEST_NOT_PENDING`; `503 SERVICE_UNAVAILABLE`.

## DELETE /users/me/friends/{user_id}

Удаление взаимной дружбы.

```http
DELETE /users/me/friends/22222222-2222-4222-8222-222222222222
Authorization: Bearer <access_token>
```

Успех: `204`, в том числе при отсутствии дружбы с существующим пользователем.

Тело ответа отсутствует.

Ошибки: ошибки сессии; `400 SELF_ACTION`; `404 PROFILE_NOT_FOUND`; `503 SERVICE_UNAVAILABLE`.

## GET /users/{id}/friends

Список друзей с учётом приватности. Авторизация необязательна; для собственного списка можно использовать `/users/me/friends` с токеном.

```http
GET /users/11111111-1111-4111-8111-111111111111/friends?limit=20
Authorization: Bearer <access_token>
```

Успех: `200`, `items` с публичными карточками и `next_cursor`. Доступ `friends` требует дружбы без блокировок в обе стороны.

Пример тела ответа:

```json
{
  "items": [
    {
      "id": "22222222-2222-4222-8222-222222222222",
      "username": "anna_books",
      "avatar_url": null
    }
  ],
  "next_cursor": null
}
```

Ошибки: `400 INVALID_LIMIT`, `400 INVALID_CURSOR`; `403 FORBIDDEN` — список закрыт; `404 PROFILE_NOT_FOUND`; `503 SERVICE_UNAVAILABLE`. При переданном токене — ошибки сессии; `/users/me/friends` без токена — `401 UNAUTHORIZED`.

## PUT /users/me/blocks/{user_id}

Блокировка пользователя: отменяет ожидающую заявку в обе стороны, запрещает новые заявки и закрывает доступ уровня `friends`. Саму дружбу и публичные данные сохраняет.

```http
PUT /users/me/blocks/22222222-2222-4222-8222-222222222222
Authorization: Bearer <access_token>
```

Успех: `204`, включая повтор блокировки.

Тело ответа отсутствует.

Ошибки: ошибки сессии; `400 SELF_ACTION`; `404 PROFILE_NOT_FOUND`; `503 SERVICE_UNAVAILABLE`.

## DELETE /users/me/blocks/{user_id}

Снятие только собственной блокировки. Встречная блокировка сохраняется.

```http
DELETE /users/me/blocks/22222222-2222-4222-8222-222222222222
Authorization: Bearer <access_token>
```

Успех: `204`, включая повтор разблокировки существующего пользователя.

Тело ответа отсутствует.

Ошибки: ошибки сессии; `400 SELF_ACTION`; `404 PROFILE_NOT_FOUND`; `503 SERVICE_UNAVAILABLE`.

## GET /users/me/blocks

Список собственных исходящих блокировок.

```http
GET /users/me/blocks?limit=20
Authorization: Bearer <access_token>
```

Успех: `200`, `items` с публичными карточками и `next_cursor`.

Пример тела ответа:

```json
{
  "items": [
    {
      "id": "22222222-2222-4222-8222-222222222222",
      "username": "anna_books",
      "avatar_url": null
    }
  ],
  "next_cursor": null
}
```

Ошибки: ошибки сессии; `400 INVALID_LIMIT`, `400 INVALID_CURSOR`; `503 SERVICE_UNAVAILABLE`.

## POST /internal/auth/introspect

Для Collection, Community, Catalog и Moderation. Bearer — секрет вызывающего сервиса; в теле — пользовательский access.

```http
POST /internal/auth/introspect
Authorization: Bearer <service_secret>
Content-Type: application/json

{"access_token":"<user_access_token>"}
```

Успех: `200 {"active":true,"user_id":"11111111-1111-4111-8111-111111111111"}` либо `200 {"active":false}` для неверного токена/недействующей сессии. Роль не выдаётся.

Пример тела ответа:

```json
{
  "active": true,
  "user_id": "11111111-1111-4111-8111-111111111111"
}
```

При недействующем токене или сессии (тоже `200`):

```json
{
  "active": false
}
```

Ошибки: служебные ошибки; `400 INVALID_JSON`; `503 SERVICE_UNAVAILABLE` при сбое БД, вместо `active:false`.

## POST /internal/auth/permission-check

Только Moderation. Проверка разрешения на рассмотрение заявок каталога.

```http
POST /internal/auth/permission-check
Authorization: Bearer <moderation_service_secret>
Content-Type: application/json

{"access_token":"<user_access_token>","permission":"catalog_submissions.review"}
```

Успех: `200 {"allowed":true}` для `platform_moderator`, иначе `200 {"allowed":false}` для обычного авторизованного пользователя.

Пример тела ответа:

```json
{
  "allowed": true
}
```

При отсутствии разрешения у авторизованного пользователя (тоже `200`):

```json
{
  "allowed": false
}
```

Ошибки: служебные ошибки и ошибки сессии; `400 INVALID_JSON`, `400 INVALID_REQUEST` (неизвестное разрешение); `403 FORBIDDEN` — вызывающий сервис не Moderation.

## POST /internal/users/access-check

Для Collection и Community. Проверяет 1–100 элементов; разделы: `profile`, `description`, `friends`, `communities`, `collection`.

```http
POST /internal/users/access-check
Authorization: Bearer <collection_service_secret>
Content-Type: application/json

{"access_token":"<user_access_token>","checks":[{"owner_id":"11111111-1111-4111-8111-111111111111","section":"profile"},{"owner_id":"11111111-1111-4111-8111-111111111111","section":"collection","visibility":"friends"}]}
```

Для `collection` обязательна `visibility` (`public`, `friends`, `private`); этот раздел разрешён только Collection. Для остальных разделов видимость берётся из User Service. Без `access_token` или с пустой строкой проверяется доступ гостя.
Успех: `200`, `items` с `owner_id`, `section`, `exists`, `allowed`, `is_friend`, `is_blocked`. Неизвестный владелец: `exists:false`, `allowed:false`.

Пример тела ответа:

```json
{
  "items": [
    {
      "owner_id": "11111111-1111-4111-8111-111111111111",
      "section": "profile",
      "exists": true,
      "allowed": true,
      "is_friend": true,
      "is_blocked": false
    },
    {
      "owner_id": "11111111-1111-4111-8111-111111111111",
      "section": "collection",
      "exists": true,
      "allowed": true,
      "is_friend": true,
      "is_blocked": false
    }
  ]
}
```

Ошибки: служебные ошибки; `400 INVALID_JSON`, `400 INVALID_REQUEST`; `403 FORBIDDEN` — сервис/раздел ему недоступен; при переданном токене — ошибки сессии. Сбой обязательной проверки — `503 SERVICE_UNAVAILABLE` для всего пакета.

## POST /internal/users/previews

Для Collection, Community, Catalog и Moderation. От 1 до 100 различных ненулевых UUID.

```http
POST /internal/users/previews
Authorization: Bearer <service_secret>
Content-Type: application/json

{"user_ids":["11111111-1111-4111-8111-111111111111","22222222-2222-4222-8222-222222222222"]}
```

Успех: `200`, `items` с публичными карточками и `not_found_ids` для отсутствующих пользователей.

Пример тела ответа:

```json
{
  "items": [
    {
      "id": "11111111-1111-4111-8111-111111111111",
      "username": "Anna",
      "avatar_url": null
    }
  ],
  "not_found_ids": [
    "22222222-2222-4222-8222-222222222222"
  ]
}
```

Ошибки: служебные ошибки; `400 INVALID_JSON`, `400 INVALID_REQUEST` — некорректный размер пакета, повторные или нулевые UUID.

## GET /internal/users/{id}/blocked-user-ids

Только Community; ID в пути должен совпадать с пользователем из `X-User-Authorization`.

```http
GET /internal/users/11111111-1111-4111-8111-111111111111/blocked-user-ids?limit=20
Authorization: Bearer <community_service_secret>
X-User-Authorization: Bearer <user_access_token>
```

Успех: `200`, `items` с UUID исходящих блокировок и `next_cursor`. Для полной фильтрации ленты нужно прочитать все страницы.

Пример тела ответа:

```json
{
  "items": [
    "22222222-2222-4222-8222-222222222222"
  ],
  "next_cursor": null
}
```

Ошибки: служебные ошибки и ошибки сессии; `400 INVALID_REQUEST` — UUID пути некорректен/нулевой; `400 INVALID_LIMIT`, `400 INVALID_CURSOR`; `403 FORBIDDEN` — неверный сервис или запрос чужих блокировок.

## GET /health

Проверка доступности процесса без обращения к БД.

```http
GET /health
```

Успех: `200 {"status":"ok"}`. Прикладных ошибок нет.

Пример тела ответа:

```json
{
  "status": "ok"
}
```

## GET /ready

Проверка PostgreSQL с таймаутом 2 секунды.

```http
GET /ready
```

Успех: `200 {"status":"ready"}`.

Пример тела ответа:

```json
{
  "status": "ready"
}
```

Ошибки: `503 SERVICE_UNAVAILABLE` — PostgreSQL недоступен или истёк таймаут проверки.
