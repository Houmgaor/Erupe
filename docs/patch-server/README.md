# Patch server helpers

Files for hosting the game-file tree that the original launcher and
[mhf-outpost](https://github.com/Mogapedia/mhf-outpost) sync from. The
procedure — what to put in the tree, which `config.json` keys point at it,
how to check it works — is in the wiki page
[Client Distribution](https://github.com/Houmgaor/Erupe/wiki/Client-Distribution).

| File | Purpose |
|------|---------|
| `genMhfKey.py` | Walks `mhfdat/` and writes the CRC32 manifest (`key00.txt`, rename to `key.txt`). Pass `51` for a PS3 tree (SHA-256). From [Mezeporta/Servers](https://github.com/Mezeporta/Servers). |
| `nginx-patch-server.conf` | nginx `location` blocks that serve `key.txt`/`chk.txt` behind `/mhf_file.php` and the files under `/mhfdat/`. Include from a plain-HTTP `server {}` block. |

```bash
cd /var/www/patch            # contains mhfdat/{exe,dat}
python3 genMhfKey.py && mv key00.txt key.txt
printf '[mhf Check Message:0]' > chk.txt
curl -s 'http://localhost/mhf_file.php?chk=1'   # → [mhf Check Message:0]
```

Erupe itself serves none of this; it only hands the addresses to clients
(`PatchServerManifest`/`PatchServerFile` for the original launcher,
`API.PatchServer` for mhf-outpost).
