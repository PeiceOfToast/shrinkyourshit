# Intel Quick Sync, plainly

This page explains what ShrinkYourShit does with Intel Quick Sync (QSV)
hardware, which extra features it probes for, what the two QSV settings
do, and — just as important — what it deliberately does not do.

## The default: constant quality (ICQ)

There are two ways to tell an encoder what you want:

- **Bitrate targeting** — "make this file about 8 megabits per second."
  The encoder spends whatever quality that budget buys: a busy action
  scene looks worse, a static dialogue scene looks better, but the file
  size is predictable.
- **Constant quality** — "make this file look this good, whatever it
  costs in bytes." The encoder spends more on hard scenes and less on
  easy ones. Quality is predictable; the file size is not.

ShrinkYourShit uses Intel's constant-quality mode (**ICQ**, intelligent
constant quality) everywhere. For a home library that is the right
default: nobody knows what bitrate a movie *should* be, everybody knows
whether it looks good. The quality slider on the easy screen maps onto
ICQ quality values (`-global_quality`): step 1 saves the most space, step
5 keeps the most quality. The mapping lives in
`internal/encoders/quality.go`, one table, documented there.

This is also what the QSV path has always done — task 35 only made it
explicit and documented. No value changed.

## The optional extras, and how they are found

Intel's driver makes several encoder extras available **optionally** —
the same CPU generation can differ by driver version. ShrinkYourShit
therefore never assumes: on start (and whenever hardware is detected) the
worker runs a one-second self-test per candidate feature, exactly the way
it tests the encoders themselves, and records what passed. A machine
where the probe cannot run (no Intel device, no ffmpeg) reports every
extra as unavailable rather than guessing.

What gets probed, and what each one is:

| Feature | What it does | The trade-off |
|---------|--------------|---------------|
| Look-ahead (with extended bitrate control) | The encoder studies a window of upcoming frames before deciding how to spend its bits on the current one. Better quality at the same file size. | Slower, and it needs more memory in flight. |
| Extended bitrate control (extbrc) | A more sophisticated rate-control algorithm; look-ahead requires it. | Slightly more work per frame. |
| Async depth | More frames kept in flight inside the encoder, so it keeps up better under load. | Higher latency; the encode takes a moment longer to react to changes. |
| Low power | The encoder's low-power silicon path (works on Ice Lake and newer). | Faster and draws less power, at slightly worse quality. |

You can see what **your** machine supports on the Nodes page: every node
card lists the probed extras in plain language, including what the
machine cannot do ("This machine cannot do the faster low-power mode.").
That is also how you answer "why did the setting do nothing?" — the
setting only applies where the probe found the feature.

One fact worth knowing: ffmpeg's look-ahead option exists only for the
**H.264** QSV encoder (its HEVC and AV1 encoders have no such option). So
look-ahead is reported and used only when the target video format is
H.264; HEVC and AV1 targets get the other extras and the plain ICQ
quality, which is already the better-quality mode for them.

## The two settings

Both live under **Encode settings → Hardware extras**, both default to
off, and with both off the encoder arguments are byte-for-byte what they
were before this feature existed.

- **Extra quality** — turns on look-ahead and extended bitrate control on
  machines whose probe supports both. A little slower, slightly better
  files.
- **Low-power mode** — passes the low-power flag where supported. Faster,
  slightly worse quality, less power draw. Does nothing on older
  hardware.

If you turn a setting on and the machine cannot do it, nothing fails: the
node logs one warning naming the feature and carries on without it. A
setting must never turn into a failed job.

The async depth is **not** a setting. It is a throughput knob, not a
quality one, and there is no value of it a normal person could reason
about. It turns on together with either QSV setting (on machines whose
probe supports it), and its value follows the quality slider: more
quality requested, shallower pipeline (deeper pipeline for the smallest
files, shallower for the best quality).

## When low-power mode is the wrong choice

Low power trades quality for speed. That is the wrong choice when quality
is the whole point — **archival** copies you want to still look good in
ten years, and any **high quality** slider step. If your goal is "shrink
the archive, keep the picture", leave low power off; use the extra
quality setting instead. Low power earns its keep on throwaway or
time-sensitive encodes — a second copy for a tablet, a fast re-encode so
tonight's recording is ready.

## What Intel "Deep Link" covers — and why it is not here

Intel markets a bundle of features under the **Deep Link** name. An
honest scope statement:

- **Hyper Encode** splits one encode across an integrated GPU and a
  discrete Arc GPU at the same time. It needs **two ffmpeg processes and
  a driver-side scheduler** to hand segments between the chips — it is
  **not implemented and is not planned** in ShrinkYourShit.
- Deep Link's **bitrate boost** and **auto bitrate** features live behind
  Intel's own driver API, not behind anything ffmpeg exposes; they are
  likewise out of reach.

What this project implements is exactly the QSV feature set ffmpeg does
expose: the ICQ quality mapping, look-ahead with extended bitrate
control, the async pipeline depth and low-power mode — each probed on
your machine before it is used. No marketing list beyond that.
