# Patch server helpers

Files for hosting the game-file tree that the original launcher and
[mhf-outpost](https://github.com/Mogapedia/mhf-outpost) sync from **on a
separate web server**. If Erupe serves the tree itself
(`API.PatchTree.Enabled: true`) none of this is needed: it generates the
manifest on startup and serves the files from the API port. The procedure — what to put in the tree, which `config.json` keys point at it,
how to check it works — is in the wiki page
[Client Distribution](https://github.com/Houmgaor/Erupe/wiki/Client-Distribution).

| File | Purpose |
|------|---------|
| `../../cmd/patchtree` | `go build ./cmd/patchtree && ./patchtree gen --root /var/www/patch` — writes `key.txt`; same output as the script below, about 4 s for a ZZ tree. |
| `genMhfKey.py` | Python equivalent (from [Mezeporta/Servers](https://github.com/Mezeporta/Servers)): writes `key00.txt`, rename to `key.txt`. Pass `51` for a PS3 tree (SHA-256), which `patchtree` does not do. |
| `nginx-patch-server.conf` | nginx `location` blocks that serve `key.txt`/`chk.txt` behind `/mhf_file.php` and the files under `/mhfdat/`. Include from a plain-HTTP `server {}` block. |

```bash
cd /var/www/patch            # contains mhfdat/{exe,dat}
python3 genMhfKey.py && mv key00.txt key.txt
printf '[mhf Check Message:0]' > chk.txt
curl -s 'http://localhost/mhf_file.php?chk=1'   # → [mhf Check Message:0]
```

Whichever server hosts the tree, Erupe hands its address to clients through
`PatchServerManifest`/`PatchServerFile` (original launcher) and
`API.PatchServer` (mhf-outpost).
