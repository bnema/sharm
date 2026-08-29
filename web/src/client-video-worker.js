import {
  ALL_FORMATS,
  BlobSource,
  BufferTarget,
  Conversion,
  Input,
  Mp4OutputFormat,
  Output,
  Quality,
  canEncodeAudio,
  canEncodeVideo,
} from 'mediabunny';

const MAX_CLIENT_INPUT_BYTES = 250 * 1024 * 1024;
const MAX_CLIENT_DURATION_SECONDS = 30 * 60;
const MAX_OUTPUT_WIDTH = 1920;
const MAX_OUTPUT_HEIGHT = 1920;
const MAX_OUTPUT_PIXELS = 1920 * 1080;

/** @type {Conversion | null} */
let activeConversion = null;

function even(value) {
  return Math.max(2, Math.floor(value / 2) * 2);
}

function boundedDimensions(width, height) {
  const scale = Math.min(
    1,
    MAX_OUTPUT_WIDTH / width,
    MAX_OUTPUT_HEIGHT / height,
    Math.sqrt(MAX_OUTPUT_PIXELS / (width * height)),
  );
  return {
    width: even(width * scale),
    height: even(height * scale),
  };
}

function errorMessage(error) {
  return error instanceof Error ? error.message : 'Client-side video encoding failed.';
}

async function prepare(file) {
  if (!(file instanceof Blob)) throw new Error('The selected video could not be read.');
  if (file.size > MAX_CLIENT_INPUT_BYTES) throw new Error('This video is too large for safe in-browser encoding.');

  const input = new Input({ source: new BlobSource(file), formats: ALL_FORMATS });
  try {
    const videoTrack = await input.getPrimaryVideoTrack();
    if (!videoTrack) throw new Error('The selected file has no video track.');

    const [videoCodec, videoDecodable, width, height, duration, audioTrack] = await Promise.all([
      videoTrack.getCodec(),
      videoTrack.canDecode(),
      videoTrack.getDisplayWidth(),
      videoTrack.getDisplayHeight(),
      input.computeDuration(),
      input.getPrimaryAudioTrack(),
    ]);
    if (!Number.isFinite(duration) || duration <= 0 || duration > MAX_CLIENT_DURATION_SECONDS) {
      throw new Error('This video is too long for safe in-browser encoding.');
    }
    if (!videoDecodable) throw new Error('This browser cannot decode the selected video.');

    let audioCodec = null;
    if (audioTrack) {
      audioCodec = await audioTrack.getCodec();
      if (!(await audioTrack.canDecode())) throw new Error('This browser cannot decode the selected audio track.');
    }

    const isMP4 = file.type === 'video/mp4' || /\.m(?:p4|4v)$/i.test(file.name || '');
    if (isMP4 && videoCodec === 'avc' && (!audioTrack || audioCodec === 'aac')) {
      self.postMessage({ type: 'direct' });
      return;
    }

    const dimensions = boundedDimensions(width, height);
    const [canEncodeAVC, canEncodeAAC] = await Promise.all([
      canEncodeVideo('avc', {
        width: dimensions.width,
        height: dimensions.height,
        hardwareAcceleration: 'prefer-hardware',
      }),
      audioTrack ? canEncodeAudio('aac', {
        numberOfChannels: Math.min(await audioTrack.getNumberOfChannels(), 2),
        sampleRate: Math.min(await audioTrack.getSampleRate(), 48000),
      }) : Promise.resolve(true),
    ]);
    if (!canEncodeAVC || !canEncodeAAC) {
      throw new Error('This browser cannot encode the required H.264/AAC output.');
    }

    const target = new BufferTarget();
    const output = new Output({
      format: new Mp4OutputFormat({ fastStart: 'in-memory' }),
      target,
    });
    const conversion = await Conversion.init({
      input,
      output,
      video: {
        codec: 'avc',
        width: dimensions.width,
        height: dimensions.height,
        fit: 'contain',
        quality: new Quality('high'),
        hardwareAcceleration: 'prefer-hardware',
        keyFrameInterval: 2,
        forceTranscode: true,
      },
      audio: audioTrack ? {
        codec: 'aac',
        numberOfChannels: Math.min(await audioTrack.getNumberOfChannels(), 2),
        sampleRate: Math.min(await audioTrack.getSampleRate(), 48000),
        quality: new Quality('high'),
        forceTranscode: true,
      } : { discard: true },
    });
    if (!conversion.isValid) {
      throw new Error('The selected tracks cannot be converted to H.264/AAC on this browser.');
    }

    activeConversion = conversion;
    conversion.onProgress = (progress) => self.postMessage({ type: 'progress', progress });
    await conversion.execute();
    if (!(target.buffer instanceof ArrayBuffer)) throw new Error('The browser produced no MP4 output.');
    self.postMessage({ type: 'encoded', buffer: target.buffer }, [target.buffer]);
  } finally {
    activeConversion = null;
    input.dispose();
  }
}

self.addEventListener('message', (event) => {
  if (event.data?.type === 'cancel') {
    if (activeConversion) void activeConversion.cancel();
    return;
  }
  if (event.data?.type !== 'prepare') return;
  void prepare(event.data.file).catch((error) => {
    self.postMessage({ type: 'fallback', error: errorMessage(error) });
  });
});
