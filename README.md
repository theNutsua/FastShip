# FastShip

**Local to production, with one tool.**

FastShip runs your application your code and the services it depends on 
from a single config file, with no Dockerfile and no container knowledge
required. The same tool, the same config, and the same commands work from
your first local run onward.

```yaml
# fastship.yaml
name: myapp
port: 3000
services:
  - postgres
```

```bash
fastship run myapp
```

That's it. FastShip detects your language, builds a minimal image from your
source, starts a PostgreSQL database, wires them together on a private
network, and lets your app reach the database by name all from those four
lines.

---

## What it does

- **No Dockerfile.** FastShip detects your language and builds an optimized
  image from your source automatically.
- **Services made simple.** List `postgres` and you get a running database
  with generated credentials, a persistent volume, and a connection string
  injected into your app — no manual setup.
- **Reach services by name.** Your app