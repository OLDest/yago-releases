const fs = require('node:fs');
const path = require('node:path');
const root = __dirname;
const mod = fs.readFileSync(path.join(root, 'go.mod'), 'utf8');
const entries = [...mod.matchAll(/^\s*([^\s()]+)\s+(v\S+)/gm)];
let out = 'YaGo third-party notices\n\nProtocol reference: SpolzerMod/yamusic-rs, commit 45335cf6058bc4beb424c3e24620312021b6ea06 (GPL-3.0-only).\n\n';
out += 'Go runtime and bundled Go fonts\n' + fs.readFileSync(path.join(root, '.tools/go/LICENSE'), 'utf8') + '\n\n';
for (const [, name, version] of entries) {
  const dir = path.join(root, '.tools/gopath/pkg/mod', `${name}@${version}`);
  const files = fs.readdirSync(dir).filter(n => /^(license|copying|notice)(\.|$)/i.test(n));
  if (!files.length) throw new Error(`Missing license: ${name}`);
  out += `\n========== ${name} ${version} ==========\n`;
  for (const file of files) out += `\n${file}\n${fs.readFileSync(path.join(dir, file), 'utf8')}\n`;
}
for (const [title, relative] of [
  ['Oboe Android audio engine', '.tools/gopath/pkg/mod/github.com/ebitengine/oto/v3@v3.5.1/internal/oboe/LICENSE-oboe'],
  ['Android NDK LLVM runtime/toolchain notices', '.tools/android-sdk/ndk/27.2.12479018/toolchains/llvm/prebuilt/windows-x86_64/NOTICE'],
]) {
  const file = path.join(root, relative);
  if (fs.existsSync(file)) out += `\n========== ${title} ==========\n${fs.readFileSync(file, 'utf8')}\n`;
}
fs.writeFileSync(path.join(root, 'THIRD_PARTY_NOTICES.txt'), out);
console.log(`Collected notices for ${entries.length} modules and Go runtime.`);
