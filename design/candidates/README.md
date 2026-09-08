# design/candidates — evaluation candidates (#2770)

Candidate token sets swept by the composited-contrast harness and laid out in
the comparison sheet. Each file is a **partial override** merged onto
`design/tokens.json`, so a candidate states only what it changes.

Palette and glass ramp are independent axes — every palette is rendered against
every ramp, and the sheet gives each its own picker.

## Why the axes are not symmetric

They are not equally powerful, and the arithmetic says so before any rendering.

The blur averages a busy photo to a mid grey. On a grey-109 composite the
*theoretical maximum* contrast is **4.06:1 with pure black** and **5.17:1 with
pure white** — so black text cannot reach 4.5:1 at all, and a foreground needs
relative luminance ≥ 0.863 (lighter than grey 240) to clear it. That is a pale
tint, not a brand colour.

**No palette can make brand-coloured text legible on glass.** Measured on the
shipped material: deep sage 1.47:1, `warning` 1.06:1, `error` 1.03:1 over that
backdrop — and they fail for every palette, because the constraint is the
background, not the ink.

What *can* fix it is the material. The shipped scrim is far too weak over bright
media: its worst-case composite is grey **175**, where even pure white manages
only **2.19:1**. Raising the scrim to 70% brings the worst case to 94 and white
to 6.48:1.

So the ramps below vary the material — the decisive variable — while the
palettes vary brand and neutral values, which matter for flat surfaces, fills,
and aesthetics. Expect the sheet to show the ramp axis moving glass legibility
and the palette axis barely touching it.

## Files

| File | Axis | What it changes |
| --- | --- | --- |
| `palette-ink-sage.json` | palette | Nothing — the shipped palette, as control |
| `palette-deeper-neutrals.json` | palette | Darker secondary/faint text, targeting the flat-surface failures |
| `palette-cool-slate.json` | palette | A different hue family, to test whether hue matters at all |
| `glass-current.json` | glass | Nothing — the shipped material, as control |
| `glass-deeper.json` | glass | Scrim 70%, sheet 10% — worst-case composite 94 |
| `glass-heaviest.json` | glass | Scrim 80%, sheet 8% — worst-case composite 67 |

## Adding one

Drop a file in here with `id`, `axis` (`palette` or `glass`), `label`, and the
subtree it overrides (`color` for a palette, `glass` for a ramp). The sweep
picks it up; nothing else needs editing.
