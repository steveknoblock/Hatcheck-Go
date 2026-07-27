You'll need three headers on every admin request — the session JWT, user ID, and your bootstrap token (or an admin capability token). Here are the curl commands:

**Assign a role**
```bash
curl -X POST "http://localhost:8090/role/assign?principal=<user-id>&role=editor&reason=initial+setup" \
  -H "Authorization: Bearer <session-jwt>" \
  -H "X-User-ID: <user-id>" \
  -H "X-Bootstrap-Token: <bootstrap-token>"
```

**Remove a role**
```bash
curl -X POST "http://localhost:8090/role/revoke?principal=<user-id>&role=editor&reason=left+project" \
  -H "Authorization: Bearer <session-jwt>" \
  -H "X-User-ID: <user-id>" \
  -H "X-Bootstrap-Token: <bootstrap-token>"
```

**List all active roles**
```bash
curl "http://localhost:8090/roles" \
  -H "Authorization: Bearer <session-jwt>" \
  -H "X-User-ID: <user-id>" \
  -H "X-Bootstrap-Token: <bootstrap-token>"
```

**Roles for a specific principal**
```bash
curl "http://localhost:8090/roles?principal=<user-id>" \
  -H "Authorization: Bearer <session-jwt>" \
  -H "X-User-ID: <user-id>" \
  -H "X-Bootstrap-Token: <bootstrap-token>"
```

**Principals holding a specific role**
```bash
curl "http://localhost:8090/roles?role=editor" \
  -H "Authorization: Bearer <session-jwt>" \
  -H "X-User-ID: <user-id>" \
  -H "X-Bootstrap-Token: <bootstrap-token>"
```

You can grab your `session-jwt` and `user-id` from `sessionStorage` in the browser devtools console:

```javascript
sessionStorage.getItem('session_jwt')
sessionStorage.getItem('user_id')
```