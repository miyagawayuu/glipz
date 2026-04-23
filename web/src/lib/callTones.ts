type ToneStep = {
  frequency: number;
  durationMs: number;
  gain: number;
};

type TonePattern = {
  attackMs?: number;
  releaseMs?: number;
  pauseMs?: number;
  steps: ToneStep[];
};

export type CallToneController = {
  start: () => Promise<void>;
  stop: () => void;
  isPlaying: () => boolean;
};

function clampMs(value: number | undefined, fallback: number): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.max(0, Number(value));
}

function createStepOscillator(
  ctx: AudioContext,
  destination: AudioNode,
  startAt: number,
  step: ToneStep,
  attackMs: number,
  releaseMs: number,
): number {
  const durationSec = Math.max(0.02, step.durationMs / 1000);
  const endAt = startAt + durationSec;
  const attackSec = Math.min(durationSec * 0.35, attackMs / 1000);
  const releaseSec = Math.min(durationSec * 0.45, releaseMs / 1000);

  const osc = ctx.createOscillator();
  const gain = ctx.createGain();

  osc.type = "sine";
  osc.frequency.setValueAtTime(step.frequency, startAt);
  gain.gain.setValueAtTime(0.0001, startAt);
  gain.gain.linearRampToValueAtTime(Math.max(0.0001, step.gain), startAt + attackSec);
  gain.gain.setValueAtTime(Math.max(0.0001, step.gain), Math.max(startAt + attackSec, endAt - releaseSec));
  gain.gain.linearRampToValueAtTime(0.0001, endAt);

  osc.connect(gain);
  gain.connect(destination);
  osc.start(startAt);
  osc.stop(endAt + 0.02);
  return endAt;
}

export function createLoopingCallTone(pattern: TonePattern): CallToneController {
  let ctx: AudioContext | null = null;
  let masterGain: GainNode | null = null;
  let loopTimer: ReturnType<typeof setTimeout> | null = null;
  let playing = false;

  const attackMs = clampMs(pattern.attackMs, 24);
  const releaseMs = clampMs(pattern.releaseMs, 120);
  const pauseMs = clampMs(pattern.pauseMs, 0);

  function clearTimer() {
    if (loopTimer) {
      clearTimeout(loopTimer);
      loopTimer = null;
    }
  }

  function scheduleCycle() {
    if (!ctx || !masterGain || !playing) return;
    const startedAt = ctx.currentTime + 0.02;
    let cursor = startedAt;
    for (const step of pattern.steps) {
      cursor = createStepOscillator(ctx, masterGain, cursor, step, attackMs, releaseMs);
    }
    const totalMs = Math.max(80, Math.ceil((cursor - startedAt) * 1000 + pauseMs));
    clearTimer();
    loopTimer = setTimeout(() => {
      if (playing) scheduleCycle();
    }, totalMs);
  }

  async function ensureContext() {
    if (typeof window === "undefined") return null;
    const AudioCtor = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!AudioCtor) return null;
    if (!ctx) {
      ctx = new AudioCtor();
    }
    if (!masterGain) {
      masterGain = ctx.createGain();
      masterGain.gain.value = 0.9;
      masterGain.connect(ctx.destination);
    }
    if (ctx.state === "suspended") {
      await ctx.resume().catch(() => undefined);
    }
    return ctx;
  }

  return {
    async start() {
      if (playing) return;
      const ready = await ensureContext();
      if (!ready || ready.state !== "running") return;
      playing = true;
      scheduleCycle();
    },
    stop() {
      playing = false;
      clearTimer();
      if (masterGain && ctx) {
        const now = ctx.currentTime;
        masterGain.gain.cancelScheduledValues(now);
        masterGain.gain.setValueAtTime(masterGain.gain.value || 0.9, now);
        masterGain.gain.linearRampToValueAtTime(0.0001, now + 0.08);
      }
    },
    isPlaying() {
      return playing;
    },
  };
}

export function createIncomingCallTone(): CallToneController {
  return createLoopingCallTone({
    attackMs: 30,
    releaseMs: 180,
    pauseMs: 1150,
    steps: [
      { frequency: 880, durationMs: 210, gain: 0.12 },
      { frequency: 1174, durationMs: 210, gain: 0.1 },
      { frequency: 880, durationMs: 210, gain: 0.12 },
      { frequency: 1174, durationMs: 210, gain: 0.1 },
    ],
  });
}

export function createOutgoingCallTone(): CallToneController {
  return createLoopingCallTone({
    attackMs: 18,
    releaseMs: 120,
    pauseMs: 1600,
    steps: [
      { frequency: 440, durationMs: 320, gain: 0.08 },
      { frequency: 554.37, durationMs: 320, gain: 0.06 },
    ],
  });
}
