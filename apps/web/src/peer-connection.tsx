import { useEffect, useRef, useState } from "react";
import { Camera, Copy, Download, QrCode } from "lucide-react";
import { api, type Client } from "./api";
import { useApp } from "./state";
import {
  Button,
  Editor,
  ErrorNote,
  Field,
  Input,
  Modal,
} from "./components/ui";
import {
  encodePeerConnection,
  parsePeerConnection,
  type PeerConnection,
} from "./peer-connection-data";

export function ClientQR({
  client,
  onClose,
}: {
  client: Client;
  onClose: () => void;
}) {
  const { status } = useApp();
  const [endpoint, setEndpoint] = useState(
    (status.gateway_url || location.origin).replace(/\/$/, "") + "/pacaroute",
  );
  const [key, setKey] = useState("");
  const [keyError, setKeyError] = useState("");
  const [image, setImage] = useState<{ url: string; payload: string }>();
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    if (!client.local)
      api<{ api_key: string }>(
        `clients/${client.id}/key`,
        "GET",
        undefined,
        controller.signal,
      )
        .then((v) => {
          if (v.api_key) setKey(v.api_key);
          else
            setKeyError(
              "旧客户端尚未保存可读取的 Key。请在接入脚本中补填原 Key，或轮换密钥后重试。",
            );
        })
        .catch(() => {
          if (!controller.signal.aborted)
            setKeyError(
              "无法读取这个客户端的 Key。旧客户端可先在接入脚本中补填原 Key，或轮换密钥后重试。",
            );
        });
    return () => controller.abort();
  }, [client.id, client.local]);
  let payload = "",
    validation = "";
  try {
    if (key)
      payload = encodePeerConnection({
        version: 1,
        endpoint,
        client_key: key,
        name: client.name,
      });
  } catch (e) {
    validation = (e as Error).message;
  }
  useEffect(() => {
    let active = true;
    setImage(undefined);
    setError("");
    setCopied(false);
    if (payload)
      import("qrcode")
        .then(({ default: QRCode }) =>
          QRCode.toDataURL(payload, {
            errorCorrectionLevel: "M",
            margin: 4,
            scale: 8,
            color: { dark: "#101820", light: "#ffffff" },
          }),
        )
        .then((url) => {
          if (active) setImage({ url, payload });
        })
        .catch(() => {
          if (active) setError("二维码生成失败，请缩短地址或名称");
        });
    return () => {
      active = false;
    };
  }, [payload]);
  const ready = image?.payload === payload ? image : undefined;
  let local = false;
  try {
    local = ["localhost", "127.0.0.1", "[::1]"].includes(
      new URL(endpoint).hostname,
    );
  } catch {
    /* Validation is shown below. */
  }
  return (
    <Modal title={client.name + " · 接入二维码"} onClose={onClose}>
      <div className="form-body">
        <Field
          label="供对方访问的服务地址"
          hint="填写对方能访问的域名或局域网地址。此处不会修改服务监听地址。"
        >
          <Input
            type="url"
            value={endpoint}
            onChange={(e) => setEndpoint(e.target.value)}
          />
        </Field>
        {local && (
          <p className="muted">
            当前是本机地址，其他设备需要改为这台服务的 IP 或域名。
          </p>
        )}
        {client.disabled && (
          <ErrorNote>此客户端已停用，启用后才能接入。</ErrorNote>
        )}
        {(keyError || error || validation) && (
          <ErrorNote>{keyError || error || validation}</ErrorNote>
        )}
        {ready ? (
          <img
            className="connection-qr"
            src={ready.url}
            alt="PacaRoute 客户端接入二维码"
          />
        ) : (
          !keyError &&
          !validation &&
          !error && <p className="muted">正在生成二维码…</p>
        )}
        <p className="muted">
          二维码和接入信息包含此客户端密钥，持有人可以使用这个客户端
          的模型。只交给对应调用方；轮换 Key 后旧二维码失效。
        </p>
        <div className="actions">
          <Button
            disabled={!ready}
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(payload);
                setCopied(true);
              } catch {
                setError("复制失败，请使用下载二维码");
              }
            }}
          >
            <Copy size={15} />
            {copied ? "已复制" : "复制接入信息"}
          </Button>
          <Button
            variant="secondary"
            disabled={!ready}
            onClick={() => {
              if (!ready) return;
              const link = document.createElement("a");
              const bytes = Uint8Array.from(
                atob(ready.url.split(",")[1]),
                (c) => c.charCodeAt(0),
              );
              const url = URL.createObjectURL(
                new Blob([bytes], { type: "image/png" }),
              );
              link.href = url;
              link.download = `PacaRoute-${client.id}-connection.png`;
              document.body.appendChild(link);
              link.click();
              link.remove();
              setTimeout(() => URL.revokeObjectURL(url), 1000);
            }}
          >
            <Download size={15} />
            下载二维码
          </Button>
        </div>
      </div>
    </Modal>
  );
}

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
      if (generation.current === token) setError((e as Error).message);
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
              setError((e as Error).message);
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
            : (e as Error).message,
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
