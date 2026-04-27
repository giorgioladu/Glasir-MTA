# Glasir-MTA

**A multi-user Mail Transfer Agent for the [Yggdrasil](https://yggdrasil-network.github.io/) overlay network.**

---

## A note of thanks

Glasir-MTA would not exist without the original work of **Neil Alexander** ([@neilalexander](https://github.com/neilalexander)) and his project [yggmail](https://github.com/neilalexander/yggmail). Yggmail was a brilliant proof of concept — a single-user MTA that proved email could work natively over Yggdrasil's encrypted overlay network, without relying on any centralised infrastructure.

We took that idea and ran with it. Thank you, Neil.

---

## What is Glasir-MTA?

Glasir-MTA is a fork and substantial rewrite of yggmail, evolved into a **multi-user, production-oriented MTA** built specifically for the Yggdrasil network.

Where yggmail was a personal experiment, Glasir aims to be something you can actually run on a node and hand accounts to your friends.

Key differences from the original yggmail:

- **Multi-user** — full user management with individual mailboxes, passwords (bcrypt), and optional quotas
- **MariaDB backend** — replaces the embedded single-user storage with a proper relational database
- **Direct IPv6 delivery** — mail is sent directly over Yggdrasil's `tun0` interface via SMTP on port 25, without spinning up a conflicting internal Yggdrasil node
- **TLS support** — encrypted SMTP and IMAP connections
- **Stamp (PoW)** — a Hashcash-inspired digital postage stamp to discourage spam on the overlay network
- **CLI** — a sysadmin-friendly command-line interface for user and alias management

---

## Requirements

| Component | Minimum version |
|-----------|----------------|
| Go | 1.24.0 |
| MariaDB | 10.11 |
| Yggdrasil | 0.5.13 |

Yggdrasil must be running as a **system daemon** before starting Glasir-MTA. Glasir does not manage the Yggdrasil connection itself — it uses the `tun0` interface that the daemon creates.

---

## Building from source

```bash
git clone https://github.com/giorgioladu/Glasir-MTA
cd Glasir-MTA
go build ./cmd/yggmail/
```

The result is a single binary called `yggmail` in the project root.

---

## Database setup

Create a database and user in MariaDB:

```sql
CREATE DATABASE glasir_db
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

CREATE USER 'glasir_user'@'%' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON glasir_db.* TO 'glasir_user'@'%';
FLUSH PRIVILEGES;
```

Then apply the schema:

```bash
mysql -u glasir_user -p glasir_db < internal/storage/mariadb/schema.sql
```

The schema creates the following tables: `users`, `mailboxes`, `messages`, `aliases`, `queue`, and `config`.

If you are upgrading from an existing installation and your `users` table does not have a `quota_bytes` column, add it:

```sql
ALTER TABLE users ADD COLUMN quota_bytes BIGINT UNSIGNED DEFAULT 0;
ALTER TABLE users MODIFY COLUMN passhash VARCHAR(255) NOT NULL;
```

---

## Configuration

Copy the example and edit it:

```bash
cp config.json.example config.json
```

```json
{
  "private_key": "your_yggdrasil_private_key_hex",
  "database": {
    "dsn": "glasir_user:your_password@tcp(127.0.0.1:3306)/glasir_db?parseTime=true",
    "max_open_conns": 10,
    "max_idle_conns": 2
  },
  "smtp": {
    "listen": "127.0.0.1:2525",
    "hostname": "your_yggdrasil_ipv6_address",
    "max_message_bytes": 10485760
  },
  "imap": {
    "listen": "127.0.0.1:143"
  }
}
```

**`private_key`** — your Yggdrasil private key in hex format. This is used only to derive your node's public key and identity (`hex@yggmail`). Glasir does not use it to manage the network connection — that is entirely handled by the Yggdrasil daemon.

You can find your private key in your Yggdrasil configuration file (usually `/etc/yggdrasil/yggdrasil.conf`).

**`smtp.hostname`** — set this to your node's Yggdrasil IPv6 address. You can find it with:

```bash
yggdrasilctl getself
```

---

## Firewall

Glasir listens on port 25 of your Yggdrasil interface (`tun0`) to receive mail from other nodes. Make sure this port is open:

```bash
# Fedora / RHEL with firewalld
sudo firewall-cmd --zone=trusted --add-interface=tun0 --permanent
sudo firewall-cmd --reload
```

Port 2525 (SMTP for local clients) and port 143 (IMAP) should be accessible only from localhost or your local network unless you know what you are doing.

---

## Running

```bash
# Start the server (requires root or CAP_NET_BIND_SERVICE for port 25)
sudo ./yggmail

# Or with a custom config path
sudo ./yggmail -config /etc/glasir/config.json
```

On startup you will see something like:

```
  ╔═══════════════════════════════════════════╗
  ║  GLASIR-MTA · Mail Transfer Agent        ║
  ║  Yggdrasil overlay  ·  v2.3              ║
  ╚═══════════════════════════════════════════╝

IMAP  listening on 127.0.0.1:143
SMTP  listening on 127.0.0.1:2525
SMTP  overlay listening on [200:xxxx:...]:25
```

---

## User management

All user management is done through the CLI. The server does not need to be running for these commands.

```bash
# Create a user (detects your Yggdrasil IP automatically)
./yggmail adduser

# List all users
./yggmail listusers

# Change a user's password
./yggmail passwd

# Delete a user and all their mail
./yggmail deluser

# Show node status and service health
./yggmail status
```

### Alias management

```bash
# List all aliases
./yggmail alias list

# Assign the node alias (hex@yggmail) to a user
./yggmail alias assign

# Create a generic alias (e.g. postmaster → alice@200:...)
./yggmail alias add -alias postmaster@200:... -target alice@200:...

# Remove an alias
./yggmail alias remove -alias postmaster@200:...
```

---

## Sending and receiving mail

Any standard IMAP/SMTP client works with Glasir. Configure it as follows:

| Setting | Value |
|---------|-------|
| SMTP server | `127.0.0.1` |
| SMTP port | `2525` |
| SMTP auth | `PLAIN` or `LOGIN` |
| IMAP server | `127.0.0.1` |
| IMAP port | `143` |
| Username | `yourname@your_yggdrasil_ipv6` |
| Password | the password you set with `adduser` |

To send mail to another Glasir-MTA node, address it as:

```
recipient@200:xxxx:xxxx:xxxx:xxxx:xxxx:xxxx:xxxx
```

where the address after `@` is the recipient's Yggdrasil IPv6 address. Both nodes must have Yggdrasil running and port 25 open on `tun0`.

---

## Stamp — Digital Postage (anti-spam)

Glasir implements a **Hashcash-inspired Proof-of-Work** system called Stamp to make bulk unsolicited mail economically unviable on the overlay network.

### How it works

Before a message is accepted for delivery, the sending server must solve a small cryptographic puzzle: find a nonce such that the SHA-256 hash of a token string has a required number of leading zero bits.

The token format is:

```
1:<bits>:<date>:<recipient>:<nonce>:<counter>
```

The result is attached to the message as a custom SMTP header:

```
X-Glasir-Stamp: 1;20;20260426;bob@201:b8f9:...:a3f9c1;482910
```

### Cost analysis

| Difficulty | Avg. attempts | Time on modern CPU | Cost for 1M emails |
|-----------|--------------|-------------------|-------------------|
| 20 bits | ~1,000,000 | ~0.5 seconds | ~140 CPU-hours |
| 22 bits | ~4,000,000 | ~2 seconds | ~555 CPU-hours |
| 24 bits | ~16,000,000 | ~8 seconds | ~2,200 CPU-hours |

For a legitimate user sending a few emails a day, the delay is completely invisible. For a spammer attempting to send millions of messages, the computational cost becomes prohibitive.

The difficulty level is configurable in `internal/stamp/stamp.go` by changing the `Bits` constant.

### Verification

Receiving nodes verify the stamp automatically in `session_ygg.go`. Messages arriving without a valid stamp are rejected before they reach the user's mailbox.

---

## TLS

Glasir supports TLS for both SMTP (port 2525) and IMAP (port 143). To enable it, provide certificate and key paths in your configuration:
Note that traffic between Glasir nodes travelling over Yggdrasil is already encrypted at the network layer. 
TLS is most useful for the local SMTP and IMAP ports, to protect credentials between your mail client and the server.

---

## Address formats

Glasir understands two address formats:

| Format | Example | Used for |
|--------|---------|----------|
| `user@ipv6` | `alice@200:7987:...:4291` | Standard delivery between nodes |
| `hex@yggmail` | `c33c4ed3...@yggmail` | Legacy yggmail compatibility; Glasir resolves the hex key to an IPv6 address automatically |

---

## Differences from yggmail

| Feature | yggmail | Glasir-MTA |
|---------|---------|------------|
| Users | Single user | Multi-user |
| Storage | Embedded | MariaDB |
| Network | Internal Yggdrasil node | System daemon + tun0 |
| Anti-spam | None | Stamp (PoW) |
| Encryption | Yggdrasil overlay | Yggdrasil + TLS |
| CLI | Minimal | Full sysadmin CLI |
| Aliases | None | Full alias management |

---

## License

Mozilla Public License 2.0 — see [LICENSE](LICENSE) for the full text.

The original yggmail code is copyright © 2021 Neil Alexander and is also licensed under MPL-2.0.

---

## Contributing and bug reports

If something is broken or behaving strangely, open an issue at [github.com/giorgioladu/Glasir-MTA](https://github.com/giorgioladu/Glasir-MTA). No promises on response time — this is a spare-time project — but all reports are read.
