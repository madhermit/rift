# Demo captures

The screenshots and GIFs embedded in the top-level README (`docs/images/`) are
generated with [VHS](https://github.com/charmbracelet/vhs) so they stay in sync
with the UI. To regenerate them all:

```bash
mise run demo
```

That builds `rift`, creates a throwaway repo with representative content
(`demo/setup.sh`), and runs each `.tape` to drive a view and write its still
(and, for `diff`, `stage`, `tests`, and `reviewed`, a short GIF) into
`docs/images/`.

## Requirements

- **`vhs`** and **`ttyd`** on your `PATH` — VHS fetches its own headless
  Chromium and uses `ffmpeg` to encode.
- **RobotoMono Nerd Font** installed — the tapes set it as the terminal font so
  rift's `❯` breadcrumb and file-type icons render. Get it from
  [Nerd Fonts](https://github.com/ryanoasis/nerd-fonts) (or change `Set
  FontFamily` in the tapes to any installed Nerd Font).
