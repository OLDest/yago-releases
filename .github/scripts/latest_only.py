"""Keep the latest complete YaGo release and its parentless public source snapshot."""
import base64
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import zipfile
from io import BytesIO

REPO = 'OLDest/yago-releases'
API = 'https://api.github.com/repos/' + REPO
TOKEN = os.environ['GH_TOKEN']
PUBLIC_KEY = '2wg1uKVpI1ZokABNuJZQ1/vS/aMEi21WgwI6vTP65n8='


def api(path, method='GET', data=None):
    """Call the repository API without exposing its job-scoped credential."""
    headers = {'Authorization': 'Bearer ' + TOKEN, 'User-Agent': 'YaGo-latest-only',
               'Accept': 'application/vnd.github+json'}
    body = None if data is None else json.dumps(data).encode()
    request = urllib.request.Request(API + path, data=body, headers=headers, method=method)
    with urllib.request.urlopen(request, timeout=90) as response:
        raw = response.read()
    return json.loads(raw) if raw else None


def pages(path):
    """Read every release or reference, including repositories with over 100 entries."""
    items = []
    for page in range(1, 1000):
        batch = api(path + '?per_page=100&page=' + str(page))
        items.extend(batch)
        if len(batch) < 100:
            return items
    raise RuntimeError('Unexpected pagination size')


def current():
    """Select the published release with the highest numeric YaGo version."""
    releases = pages('/releases')
    published = [r for r in releases if not r['draft'] and not r['prerelease']
                 and re.fullmatch(r'v\d+\.\d+\.\d+', r['tag_name'])]
    if not published:
        raise RuntimeError('No stable YaGo release')
    latest = max(published, key=lambda r: tuple(map(int, r['tag_name'][1:].split('.'))))
    return latest, releases


def download(asset):
    """Download a public release asset and verify its GitHub digest when present."""
    request = urllib.request.Request(asset['browser_download_url'], headers={'User-Agent': 'YaGo-latest-only'})
    with urllib.request.urlopen(request, timeout=120) as response:
        raw = response.read()
    if len(raw) != asset['size']:
        raise RuntimeError('Incomplete asset: ' + asset['name'])
    if asset.get('digest') and asset['digest'] != 'sha256:' + hashlib.sha256(raw).hexdigest():
        raise RuntimeError('Asset digest mismatch: ' + asset['name'])
    return raw


def git(work, *args, input=None):
    """Run Git in the isolated snapshot directory with failures stopping publication."""
    return subprocess.check_output(['git', *args], cwd=work, input=input).decode().strip()


def enforce():
    """Verify release completeness before replacing references or deleting older downloads."""
    latest, _ = current()
    tag = latest['tag_name']
    version = tag[1:]
    required = {'YaGo.exe', 'YaGo-android.apk', 'SHA256SUMS.txt', 'updates.json',
                'LICENSE', 'THIRD_PARTY_NOTICES.txt', f'YaGo-{version}-source.zip'}
    assets = {a['name']: a for a in latest['assets']}
    if not required.issubset(assets) or any(a['state'] != 'uploaded' for a in latest['assets']):
        raise RuntimeError('Latest release assets are not complete yet')
    content = {name: download(assets[name]) for name in required}
    sums = dict((line.split(maxsplit=1)[1].strip().lstrip('*'), line.split()[0])
                for line in content['SHA256SUMS.txt'].decode().splitlines() if line.strip())
    for name in required - {'SHA256SUMS.txt'}:
        if sums.get(name) != hashlib.sha256(content[name]).hexdigest():
            raise RuntimeError('SHA256SUMS mismatch: ' + name)
    envelope = json.loads(content['updates.json'])
    payload = base64.b64decode(envelope['payload'], validate=True)
    manifest = json.loads(payload)
    if manifest['repo'] != REPO or manifest['version'] != version:
        raise RuntimeError('Update manifest version mismatch')
    for item in manifest['artifacts'].values():
        raw = content[item['name']]
        if len(raw) != item['size'] or hashlib.sha256(raw).hexdigest() != item['sha256']:
            raise RuntimeError('Update artifact mismatch')

    with tempfile.TemporaryDirectory(prefix='yago-snapshot-') as temporary:
        work = Path(temporary)
        # Verify the existing pinned update key before trusting the new release.
        key = work / 'public.der'
        key.write_bytes(bytes.fromhex('302a300506032b6570032100') + base64.b64decode(PUBLIC_KEY))
        (work / 'payload').write_bytes(payload)
        (work / 'signature').write_bytes(base64.b64decode(envelope['signature'], validate=True))
        subprocess.run(['openssl', 'pkeyutl', '-verify', '-pubin', '-keyform', 'DER',
                        '-inkey', str(key), '-rawin', '-in', str(work / 'payload'),
                        '-sigfile', str(work / 'signature')], check=True)
        for path in (key, work / 'payload', work / 'signature'):
            path.unlink()
        source_blobs = {}
        with zipfile.ZipFile(BytesIO(content[f'YaGo-{version}-source.zip'])) as archive:
            for item in archive.infolist():
                if item.is_dir():
                    continue
                path = PurePosixPath(item.filename)
                if path.is_absolute() or '..' in path.parts or '.git' in path.parts or '\\' in item.filename:
                    raise RuntimeError('Unsafe source path')
                if (item.external_attr >> 16) & 0o170000 == 0o120000:
                    raise RuntimeError('Source archive contains a symlink')
                relative = path.relative_to('YaGo-' + version)
                name = 'source/' + relative.as_posix()
                if name in source_blobs:
                    raise RuntimeError('Duplicate source path')
                raw = archive.read(item)
                source_blobs[name] = hashlib.sha1(('blob ' + str(len(raw)) + '\0').encode() + raw).hexdigest()
                destination = work / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(raw)
        if 'source/main.go' not in source_blobs or 'source/go.mod' not in source_blobs:
            raise RuntimeError('Missing application source')
        original = api('/git/ref/heads/main')['object']['sha']
        tree = api('/git/trees/' + original + '?recursive=1')
        if tree.get('truncated'):
            raise RuntimeError('Repository tree was truncated')
        # Preserve the landing page, retention workflow and all unrelated root files.
        for item in tree['tree']:
            name = item['path']
            if item['type'] != 'blob' or name.startswith('source/'):
                continue
            blob = api('/git/blobs/' + item['sha'])
            path = work / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(base64.b64decode(blob['content']))
            if item['mode'] == '100755':
                path.chmod(0o755)
        readme = work / 'README.md'
        text = readme.read_text(encoding='utf-8')
        text = re.sub(r'(?<![\d.])0\.\d+\.\d+(?![\d.])', version, text)
        text = re.sub(r'Все \d+ файлов', 'Все ' + str(len(source_blobs)) + ' файлов', text)
        readme.write_text(text, encoding='utf-8')
        git(work, 'init', '-b', 'docs/yago-store-privacy')
        git(work, 'config', 'core.autocrlf', 'false')
        git(work, 'config', 'user.name', 'github-actions[bot]')
        git(work, 'config', 'user.email', '41898282+github-actions[bot]@users.noreply.github.com')
        git(work, 'add', '-f', '.')
        snapshot_tree = git(work, 'write-tree')
        parent = api('/git/commits/' + original)
        commit = original
        if snapshot_tree != parent['tree']['sha'] or parent['parents']:
            git(work, 'commit', '-m', 'Publish YaGo ' + tag + ' source snapshot')
            commit = git(work, 'rev-parse', 'HEAD')
        refs = pages('/git/refs/tags')
        # A newer publication cancels this run before any destructive action.
        if current()[0]['id'] != latest['id']:
            raise RuntimeError('A newer release appeared; retry with it')
        if api('/git/ref/heads/main')['object']['sha'] != original:
            raise RuntimeError('Main changed during preparation; retry')
        tag_ref = next((ref for ref in refs if ref['ref'] == 'refs/tags/' + tag), None)
        if commit != original:
            git(work, 'remote', 'add', 'origin', 'https://github.com/' + REPO + '.git')
            auth = base64.b64encode(('x-access-token:' + TOKEN).encode()).decode()
            # Git receives the credential through the environment, never a URL or log.
            os.environ['GIT_CONFIG_COUNT'] = '1'
            os.environ['GIT_CONFIG_KEY_0'] = 'http.https://github.com/.extraheader'
            os.environ['GIT_CONFIG_VALUE_0'] = 'AUTHORIZATION: basic ' + auth
            push = ['push', '--atomic', 'origin', '--force-with-lease=refs/heads/main:' + original]
            push += ['--force-with-lease=refs/tags/' + tag + ':' + (tag_ref['object']['sha'] if tag_ref else '')]
            git(work, *push, 'HEAD:refs/heads/main', 'HEAD:refs/tags/' + tag)
        else:
            if tag_ref:
                if tag_ref['object']['sha'] != commit:
                    api('/git/refs/tags/' + tag, 'PATCH', {'sha': commit, 'force': True})
            else:
                api('/git/refs', 'POST', {'ref': 'refs/tags/' + tag, 'sha': commit})
        published = api('/git/trees/' + commit + '?recursive=1')
        actual = {item['path']: item['sha'] for item in published['tree'] if item['type'] == 'blob' and item['path'].startswith('source/')}
        if actual != source_blobs or api('/git/commits/' + commit)['parents']:
            raise RuntimeError('Published source snapshot verification failed')
        # Only now remove older versions; never delete drafts or a concurrent new release.
        if current()[0]['id'] != latest['id']:
            raise RuntimeError('A newer release appeared; old releases retained')
        for release in pages('/releases'):
            if release['id'] != latest['id'] and not release['draft'] and release['created_at'] <= latest['created_at']:
                api('/releases/' + str(release['id']), 'DELETE')
        for ref in refs:
            if ref['ref'] != 'refs/tags/' + tag:
                api('/git/' + ref['ref'], 'DELETE')
        releases = [r for r in pages('/releases') if not r['draft']]
        if len(releases) != 1 or releases[0]['id'] != latest['id'] or len(pages('/git/refs/tags')) != 1:
            raise RuntimeError('Latest-only retention verification failed')
        before = [(a['id'], a['size'], a.get('digest')) for a in latest['assets']]
        after = [(a['id'], a['size'], a.get('digest')) for a in api('/releases/' + str(latest['id']))['assets']]
        if before != after:
            raise RuntimeError('Latest release assets changed during cleanup')
        print('Verified:', tag, commit, len(source_blobs), 'source files; one root commit, release and tag')


if __name__ == '__main__':
    for attempt in range(30):
        try:
            enforce()
            break
        except (RuntimeError, urllib.error.URLError, subprocess.CalledProcessError) as error:
            if attempt == 29:
                raise
            print('Publication not ready; retrying:', type(error).__name__)
            time.sleep(15)
