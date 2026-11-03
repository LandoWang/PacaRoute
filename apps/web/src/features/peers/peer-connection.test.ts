import jsQR from "jsqr";
import QRCode from "qrcode";
import { describe, it, expect } from "vitest";
import {
  compatibleAccount,
  resourceKind,
  peerCapabilities,
} from "../../domain/accounts";
import {
  encodePeerConnection,
  parsePeerConnection,
  normalizePeerEndpoint,
} from "./peer-connection-data";
describe("PacaRoute 客户端 handoff", () => {
  const connection = {
    version: 1 as const,
    endpoint: "https://router.example.test/team",
    client_key: "synthetic-client-only-key",
    name: "工作 · PacaRoute",
  };
  it("roundtrips through actual QR pixels, including Unicode and a deployment path", () => {
    const payload = encodePeerConnection(connection);
    const qr = QRCode.create(payload, { errorCorrectionLevel: "M" });
    const scale = 6,
      margin = 4,
      size = (qr.modules.size + margin * 2) * scale;
    const pixels = new Uint8ClampedArray(size * size * 4).fill(255);
    for (let row = 0; row < qr.modules.size; row++)
      for (let col = 0; col < qr.modules.size; col++) {
        if (!qr.modules.get(row, col)) continue;
        for (let y = 0; y < scale; y++)
          for (let x = 0; x < scale; x++) {
            const offset =
              (((row + margin) * scale + y) * size +
                (col + margin) * scale +
                x) *
              4;
            pixels[offset] = pixels[offset + 1] = pixels[offset + 2] = 0;
          }
      }
    const decoded = jsQR(pixels, size, size);
    expect(decoded).not.toBeNull();
    expect(parsePeerConnection(decoded!.data)).toEqual(connection);
    expect(parsePeerConnection(JSON.stringify(connection))).toEqual(connection);
  });
  it("rejects unsafe, malformed, future and credential-contaminated payloads", () => {
    for (const value of [
      "javascript:alert(1)",
      "https://user:password@host.test",
      "https://host.test?key=secret",
      "https://host.test#secret",
      "file:///tmp/key",
    ])
      expect(() => normalizePeerEndpoint(value)).toThrow();
    for (const value of [
      "https://host.test",
      "pacaroute://connect#%%%",
      JSON.stringify({ ...connection, version: 2 }),
      JSON.stringify({ ...connection, admin_key: "secret" }),
      JSON.stringify({ ...connection, client_key: "bad\nkey" }),
      "x".repeat(4097),
    ])
      expect(() => parsePeerConnection(value)).toThrow();
    expect(normalizePeerEndpoint("http://127.0.0.1:8766/")).toBe(
      "http://127.0.0.1:8766",
    );
  });
  it("keeps peer account credentials and per-Agent model capabilities separate", () => {
    expect(resourceKind("pacaroute")).toBe("pacaroute");
    expect(compatibleAccount("pacaroute", "api-key")).toBe(false);
    expect(compatibleAccount("pacaroute", "pacaroute")).toBe(true);
    expect(
      peerCapabilities([
        {
          id: "codex-only",
          name: "Coding",
          vendor: "PacaRoute",
          codex: true,
          claude: false,
        },
      ]),
    ).toEqual({ codex: ["codex-only"], "claude-code": [] });
  });
});
