// Render the same script used by the UI. Secrets arrive and leave on pipes only.
import { readFileSync } from 'node:fs';
import { build } from '../apps/web/node_modules/esbuild/lib/main.js';
const result = await build({ entryPoints: ['apps/web/src/connection-script.ts'], bundle: true, write: false, format: 'esm', platform: 'node' });
const { connectionScript } = await import('data:text/javascript;base64,' + Buffer.from(result.outputFiles[0].text).toString('base64'));
const { client, agent, url, model, key } = JSON.parse(readFileSync(0, 'utf8'));
process.stdout.write(connectionScript(client, agent, url, model, 'shell', key));
