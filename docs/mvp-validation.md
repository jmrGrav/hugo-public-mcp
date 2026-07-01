# MVP Validation

## Success criteria

The MVP is valid when all of the following are true:

1. The server starts and indexes a Hugo `public/` directory.
2. Read-only tools return only published content.
3. Search is limited to indexed metadata.
4. No client can supply a file path.
5. No request can reach the Hugo source repo.
6. No draft content is exposed.
7. The service runs behind a reverse proxy without changing the security model.

## Validation checklist

### Indexing

- Startup builds a complete index of public pages.
- The index contains slug, title, summary, canonical URL, language, date, tags, categories, and type.
- Missing or malformed content is skipped safely.

### Read-only behavior

- `list_pages` returns indexed pages only.
- `get_page` returns content only for an indexed slug.
- `search_pages` only searches index fields.
- `get_recent_posts` uses the index, not filesystem scanning.
- `list_tags` and `list_categories` derive from the index.
- `get_sitemap` and `get_feed` expose published discovery files read-only.

### Security

- traversal strings are rejected
- absolute paths are rejected
- symlinks are rejected
- drafts are not visible
- logs do not include secrets or private paths
- the service does not mutate the site

## Recommended test flow

1. Start the service against a fixture Hugo `public/` tree.
2. Run the security tests.
3. Run the MCP tool tests.
4. Run proxy-level validation with HTTPS in front.
5. Validate a real Hugo site such as `arleo.eu` as a configuration example only.

