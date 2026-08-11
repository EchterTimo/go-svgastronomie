# .githooks

To use these Git hooks, you need to set your local hooks path to point to the `.githooks` directory.

```bash
git config core.hooksPath .githooks
```

If you wish to revert to the default hooks path, you can run:

```bash
git config --unset core.hooksPath
```

If you want to disable the hooks for a single push, you can use the `--no-verify` flag:

```bash
git push --no-verify
```
