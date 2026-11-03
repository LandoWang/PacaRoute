import { Camera, QrCode } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button, Editor, ErrorNote, Field, Input } from "../../components/ui";
import { errorMessage } from "../../domain/errors";
import {
  parsePeerConnection,
  type PeerConnection,
} from "./peer-connection-data";
export function PeerScan({
  onRead,
  onClose,
}: {
  onRead: (v: PeerConnection) => void;
  onClose: () => void;
}) {
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [camera, setCamera] = useState(false);
  const [busy, setBusy] = useState(false);
  const video = useRef<HTMLVideoElement>(null);
  const stream = useRef<MediaStream | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const generation = useRef(0);
  function stop() {
    generation.current++;
    clearTimeout(timer.current);
    stream.current?.getTracks().forEach((track) => track.stop());
    stream.current = null;
    if (video.current) video.current.srcObject = null;
  }
  useEffect(() => () => stop(), []);
  function read(value: string) {
    const connection = parsePeerConnection(value);
    stop();
    onRead(connection);
  }
  async function scanImage(file?: File) {
    if (!file) return;
    stop();
    setCamera(false);
    setBusy(true);
    setError("");
    const token = generation.current;
    try {
      if (file.size > 16 * 1024 * 1024)
        throw Error("请选择小于 16 MB 的二维码图片");
      const bitmap = await createImageBitmap(file);
      try {
        const canvas = document.createElement("canvas");
        const scale = Math.min(1, 2048 / Math.max(bitmap.width, bitmap.height));
        canvas.width = Math.round(bitmap.width * scale);
        canvas.height = Math.round(bitmap.height * scale);
        const ctx = canvas.getContext("2d", { willReadFrequently: true })!;
        ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
        const { default: jsQR } = await import("jsqr");
        const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height);
        const result = jsQR(pixels.data, pixels.width, pixels.height);
        if (!result)
          throw Error("未识别到二维码，请使用完整、清晰的二维码图片");
        if (generation.current === token) read(result.data);
      } finally {
        bitmap.close();
      }
    } catch (e) {
      if (generation.current === token) setError(errorMessage(e));
    } finally {
      if (generation.current === token) setBusy(false);
    }
  }
  async function startCamera() {
    stop();
    setError("");
    setCamera(true);
    const token = generation.current;
    try {
      if (!navigator.mediaDevices?.getUserMedia)
        throw Error(
          "此浏览器无法使用摄像头。请使用 HTTPS / localhost，或上传二维码图片。",
        );
      const media = await navigator.mediaDevices.getUserMedia({
        video: { facingMode: "environment", width: { ideal: 1280 } },
        audio: false,
      });
      if (generation.current !== token) {
        media.getTracks().forEach((track) => track.stop());
        return;
      }
      stream.current = media;
      if (!video.current) {
        stop();
        return;
      }
      video.current.srcObject = media;
      await video.current.play();
      const { default: jsQR } = await import("jsqr");
      const canvas = document.createElement("canvas");
      const ctx = canvas.getContext("2d", { willReadFrequently: true })!;
      function tick() {
        if (generation.current !== token) return;
        const v = video.current;
        if (v && v.readyState >= 2 && v.videoWidth) {
          const scale = Math.min(
            1,
            1280 / Math.max(v.videoWidth, v.videoHeight),
          );
          canvas.width = Math.round(v.videoWidth * scale);
          canvas.height = Math.round(v.videoHeight * scale);
          ctx.drawImage(v, 0, 0, canvas.width, canvas.height);
          const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height);
          const result = jsQR(pixels.data, pixels.width, pixels.height);
          if (result) {
            try {
              read(result.data);
              return;
            } catch (e) {
              setError(errorMessage(e));
            }
          }
        }
        timer.current = setTimeout(tick, 350);
      }
      tick();
    } catch (e) {
      if (generation.current === token) {
        stop();
        setCamera(false);
        setError(
          e instanceof DOMException
            ? "无法打开摄像头，请检查浏览器权限或上传二维码图片。"
            : errorMessage(e),
        );
      }
    }
  }
  return (
    <Editor
      title="扫描 PacaRoute 接入二维码"
      onClose={() => {
        stop();
        onClose();
      }}
      saveLabel="读取并检查"
      onSave={async () => read(text)}
    >
      <p className="muted">识别后会回到账号表单，检查地址和名称后再添加。</p>
      <Field label="二维码图片">
        <Input
          type="file"
          accept="image/*"
          disabled={busy}
          onChange={(e) => {
            void scanImage(e.target.files?.[0]);
            e.target.value = "";
          }}
        />
      </Field>
      <Button
        type="button"
        variant="secondary"
        disabled={busy}
        onClick={() =>
          camera ? (stop(), setCamera(false)) : void startCamera()
        }
      >
        <Camera size={16} />
        {camera ? "关闭摄像头" : "用摄像头扫描"}
      </Button>
      {camera && (
        <video
          ref={video}
          className="connection-camera"
          muted
          playsInline
          aria-label="二维码扫描摄像头"
        />
      )}
      {busy && <p className="muted">正在识别二维码…</p>}
      <Field
        label="粘贴接入信息"
        hint="也可以粘贴对方复制的 PacaRoute 接入信息。"
      >
        <Input
          type="password"
          autoComplete="off"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="pacaroute://connect#…"
        />
      </Field>
      {error && <ErrorNote>{error}</ErrorNote>}
      <p className="muted">
        <QrCode size={14} /> 图片只在当前浏览器解析。
      </p>
    </Editor>
  );
}
