// Local release signing. Private key must stay outside Git and release assets.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const [dir,version,keyFile='.tools/update-signing/private.pem']=process.argv.slice(2);
if(!dir||!/^\d+\.\d+\.\d+$/.test(version||''))throw Error('Usage: rtk proxy node scripts/sign-update.cjs RELEASE_DIR VERSION [PRIVATE_KEY]');
const key=crypto.createPrivateKey(fs.readFileSync(keyFile));
const publicKey=crypto.createPublicKey(key).export({type:'spki',format:'der'}).subarray(-32).toString('base64');
if(!fs.readFileSync('internal/updates/updates.go','utf8').includes(publicKey))throw Error('Signing key does not match the pinned app key');
const artifacts={};
for(const [platform,name] of Object.entries({windows_amd64:'YaGo.exe',android_universal:'YaGo-android.apk'})){
 const full=path.join(dir,name),b=fs.readFileSync(full);
 if(!b.length||b.length>128*1024*1024)throw Error('Invalid artifact size');
 artifacts[platform]={name,size:b.length,sha256:crypto.createHash('sha256').update(b).digest('hex')};
}
const payload=Buffer.from(JSON.stringify({schema:1,repo:'OLDest/yago-releases',version,artifacts}));
const envelope={payload:payload.toString('base64'),signature:crypto.sign(null,payload,key).toString('base64')};
fs.writeFileSync(path.join(dir,'updates.json'),JSON.stringify(envelope,null,2)+'\n');
console.log('Signed '+version+'; private key not copied');
