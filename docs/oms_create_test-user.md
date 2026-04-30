## oms create test-user

Create a test user on a Codesphere database

### Synopsis

Creates a test user with a hashed password and API token directly in a Codesphere
PostgreSQL database. The user can be used for automated smoke tests.

The command connects to the specified PostgreSQL instance and creates the necessary
database records (credentials, email confirmation, team, team membership, API token).

Credentials are displayed and saved to the OMS workdir as test-user.json.

Required environment variables:
  OMS_CS_TEST_USER_PASSWORD         Plaintext password for the test user.
  OMS_CS_TEST_USER_PASSWORD_HASHED  SHA-256 hex hash of the plaintext password.

```
oms create test-user [flags]
```

### Options

```
  -h, --help                       help for test-user
      --postgres-db string         PostgreSQL database name (default "codesphere")
      --postgres-host string       PostgreSQL host address (required)
      --postgres-password string   PostgreSQL password (required)
      --postgres-port int          PostgreSQL port (default 5432)
      --postgres-user string       PostgreSQL username (default "postgres")
      --ssl-mode string            PostgreSQL SSL mode (default "disable")
```

### SEE ALSO

* [oms create](oms_create.md)	 - Create resources for Codesphere

