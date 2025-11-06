# Running tests (web)

This project uses Vitest + Testing Library for unit and component tests.

Run tests once:

```bash
npm run test --prefix web
```

Run tests in watch mode:

```bash
npm run test:watch --prefix web
```

Run tests and produce coverage (text + lcov):

```bash
npm run test:coverage --prefix web
```

Coverage report (`lcov.info`) will be written to `web/coverage/lcov.info` and HTML reports to `web/coverage` (depending on Vitest config).

Notes
- The CI workflow runs on PRs against `main` and uploads `web/coverage/lcov.info` as an artifact.
- If you need to send coverage to a service such as Codecov, add the relevant step and set the token in repository secrets.
