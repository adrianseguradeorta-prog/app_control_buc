"""Developer-only tests. The delivered HTML needs neither Python nor a server.
Uses real browser FileSystemDirectoryHandle/createWritable on isolated OPFS;
the OS folder-picker alone is substituted. Real corporate SMB policy is untested.
"""
import base64
import importlib.util
import contextlib
import hashlib
import http.server
import io
import json
import pathlib
import threading
import tempfile
import unittest
import time
import os
import shutil
import zipfile
from playwright.sync_api import sync_playwright, Error as PlaywrightError

ROOT = pathlib.Path(__file__).resolve().parents[1]
REPO = ROOT.parent

class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *args): pass
    def __init__(self, *args, **kwargs): super().__init__(*args, directory=str(ROOT), **kwargs)

class BrowserTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), QuietHandler)
        threading.Thread(target=cls.server.serve_forever, daemon=True).start()
        cls.base = f'http://127.0.0.1:{cls.server.server_port}'
        cls.p = sync_playwright().start()
        cls.browser = cls.p.chromium.launch(executable_path=os.environ.get('CB_CHROMIUM') or shutil.which('chromium') or None, headless=True, args=['--no-sandbox'])
    @classmethod
    def tearDownClass(cls):
        cls.browser.close(); cls.p.stop(); cls.server.shutdown(); cls.server.server_close()
    def setUp(self):
        self.context = self.browser.new_context(accept_downloads=True)
        self.context.add_init_script('window.showDirectoryPicker = async () => navigator.storage.getDirectory();')
        self.page = self.context.new_page()
        self.errors = []; self.outbound = []
        self.page.on('pageerror', lambda e: self.errors.append(str(e)))
        self.context.route('**/*', lambda r: r.continue_() if r.request.url.startswith(self.base) else (self.outbound.append(r.request.url), r.abort()))
        self.page.goto(self.base+'/ABRIR.html')
    def tearDown(self):
        try:
            self.assertEqual(self.errors, [])
            self.assertEqual(self.outbound, [])
        finally: self.context.close()
    def poll(self, expression, page=None):
        page=page or self.page
        deadline=time.monotonic()+15
        while time.monotonic()<deadline:
            if page.evaluate('() => ('+expression+')'): return
            time.sleep(.02)
        raise AssertionError('Browser condition timed out: '+expression+'; '+page.locator('#status').inner_text())
    def idle(self): self.poll("!document.body.hasAttribute('aria-busy')")
    def click(self, selector): self.page.locator(selector).click(); self.idle()
    def confirm(self): self.page.locator('#confirmation button[value=ok]').click(); self.idle()
    def begin(self):
        self.page.locator('#accept').check(); self.page.locator('#connect').click(); self.idle()
    def create(self):
        self.begin(); self.page.locator('[data-action=create]').click(); self.confirm()
    def login(self, name='admin', password='Zocotroco$01'):
        self.page.locator('[name=username]').fill(name); self.page.locator('[name=password]').fill(password)
        self.page.locator('form[data-kind=login] button').click(); self.idle()
    def data(self):
        return self.page.evaluate("async()=>JSON.parse(await(await(await(await navigator.storage.getDirectory()).getDirectoryHandle('data')).getFileHandle('store.json')).getFile().then(f=>f.text()))")
    def write_data(self, obj):
        self.page.evaluate("async value=>{const d=await(await navigator.storage.getDirectory()).getDirectoryHandle('data',{create:true});const h=await d.getFileHandle('store.json',{create:true});const w=await h.createWritable();await w.write(JSON.stringify(value));await w.close()}", obj)
    def navigate(self, name): self.click(f'#nav [data-action="{name}"]')
    def submit(self, kind): self.page.locator(f'form[data-kind="{kind}"] button[type=submit], form[data-kind="{kind}"] > button:not([type])').first.click(); self.idle()
    def new_diver(self, id='TEST1'):
        self.navigate('divers'); self.click('[data-action=diverForm]')
        self.page.locator('[name=id]').fill(id);self.page.locator('[name=name]').fill('Persona ficticia <script>alert(1)</script>')
        for q in ['qual_basic','qual_sds_air','qual_sds_heliox']: self.page.locator(f'[name={q}]').check()
        self.submit('diver')
    def test_01_rules_hashes_and_boundaries(self):
        expected=hashlib.pbkdf2_hmac('sha256',b'test-password',b'test-salt',120000).hex()
        got=self.page.evaluate("()=>CBModel.hash('test-password','test-salt')")
        self.assertEqual(got,expected)
        result=self.page.evaluate("""()=>{
 const M=CBModel,d={id:'B',qual_basic:true,qual_sds_air:true,qual_sds_heliox:true};
 const basic={id:1,diver_id:'B',date:'2025-10-27',qualification:'Autónomo',gas:'AIRE',max_depth:26,bottom_time:20,valid:true};
 const test=(change)=>M.maintenance(d,[{...basic,...change}],'2026-10-27')[0];
 const air={...basic,date:'2026-10-01',qualification:'SDS aire',surface_supply:true};
 return {codes:M.ib.map(x=>M.code(x[1])),inclusive:test({}).count,expired:test({date:'2025-10-26'}).count,future:test({date:'2026-10-28'}).count,invalid:test({date:'bad'}).count,none:test({qualification:''}).count,sds:M.maintenance(d,[air,{...air,surface_supply:false}],'2026-10-27')[1].status};} """)
        self.assertEqual(len(set(result['codes'])),11);self.assertNotIn('65001',result['codes'])
        self.assertEqual(result['inclusive'],1)
        for k in ['expired','future','invalid','none']:self.assertEqual(result[k],0)
        self.assertNotEqual(result['sds'],'OK')
    def test_02_crud_reports_backup_restore(self):
        self.create();self.login();self.new_diver()
        self.assertIn('<script>',self.page.locator('#app h2').inner_text())
        self.click('[data-action=diverForm]');self.page.locator('[name=name]').fill('Persona editada')
        png=self.page.evaluate("()=>{const c=document.createElement('canvas');c.width=2;c.height=2;return c.toDataURL('image/png').split(',')[1]}")
        self.page.locator('[name=photo]').set_input_files({'name':'test.png','mimeType':'image/png','buffer':base64.b64decode(png)})
        self.submit('diver');self.poll("document.querySelector('img.photo')?.naturalWidth===2")
        self.navigate('dives');self.click('[data-action=diveForm]')
        self.page.locator('[name=date]').fill('2026-01-15');self.page.locator('[name=max_depth]').fill('26');self.page.locator('[name=bottom_time]').fill('20')
        for label,code in [('SDS Aire','62001'),('Cerrado Oxígeno','63001'),('Autónomo Aire','61001')]:
            self.page.locator('[name=dive_type]').select_option(label);self.assertEqual(self.page.locator('[name=ib_code]').input_value(),code)
        self.page.locator('[name=ib_code]').evaluate("e=>e.value='65001'")
        self.submit('dive');self.assertEqual(self.data()['dives'][0]['ib_code'],'61001')
        self.click('[data-action=diveForm][data-value="1"]');self.page.locator('[name=bottom_time]').fill('25');self.submit('dive');self.assertEqual(self.data()['dives'][0]['bottom_time'],25)
        self.navigate('maintenance');self.assertEqual(self.page.locator('#maintenance tbody tr').count(),1)
        self.navigate('monthly');self.page.locator('[name=year]').fill('2026');self.page.locator('[name=month]').select_option('1');self.submit('monthlyFilter')
        self.assertEqual(self.page.locator('#monthly-rows tr').count(),1)
        self.assertFalse(self.page.locator('#monthly-rows').evaluate('e=>e.isContentEditable'))
        self.page.locator('[data-report=title]').fill('Título de prueba')
        self.page.locator('[data-report=intro_template]').fill('Durante ENERO de 2026 · Unidad local')
        self.page.locator('form[data-kind=template] [name=name]').fill('Formato de prueba')
        self.page.locator('[name=font_family]').select_option('Calibri');self.submit('template')
        stored=self.data()['report_templates'][-1];self.assertEqual(stored['title'],'Título de prueba');self.assertIn('{{MES}}',stored['intro_template'])
        self.page.locator('[name=year]').fill('2027');self.page.locator('[name=month]').select_option('4');self.submit('monthlyFilter')
        self.assertIn('ABRIL',self.page.locator('[data-report=intro_template]').inner_text())
        self.page.pdf(path=str(pathlib.Path(tempfile.gettempdir())/'browser-edition-report.pdf'),format='A4')
        self.navigate('divers');self.click('[data-action=diver]');self.click('[data-action=individual]');self.click('[data-action=togglePhoto]');self.assertTrue(self.page.locator('#photo').is_hidden())
        self.navigate('admin')
        with self.page.expect_download() as d:self.click('[data-action=downloadBackup]')
        backup=pathlib.Path(d.value.path()).read_bytes()
        if os.environ.get('CB_TEST_ARTIFACTS'):
            artifacts=pathlib.Path(os.environ['CB_TEST_ARTIFACTS']);artifacts.mkdir(parents=True,exist_ok=True);(artifacts/'generated-backup.zip').write_bytes(backup)
        with zipfile.ZipFile(io.BytesIO(backup)) as z:
            self.assertIn('manifest.json',z.namelist());self.assertTrue(any(n.startswith('data/photos/') for n in z.namelist()))
        self.navigate('divers');self.click('[data-action=diver]');self.page.locator('[data-action=deleteDiver]').click();self.confirm()
        self.assertEqual(self.data()['divers'],[]);self.assertEqual(self.data()['dives'],[])
        self.navigate('admin');self.page.locator('[name=backup]').set_input_files({'name':'backup.zip','mimeType':'application/zip','buffer':backup})
        self.page.locator('form[data-kind=restore] button').click();self.page.locator('#confirmation').wait_for(state='visible');self.confirm();self.login()
        self.assertEqual(len(self.data()['divers']),1);self.assertEqual(len(self.data()['dives']),1)
        if os.environ.get('CB_TEST_ARTIFACTS'):(pathlib.Path(os.environ['CB_TEST_ARTIFACTS'])/'restored-store.json').write_text(json.dumps(self.data()))
        self.navigate('divers');self.click('[data-action=diver]');self.poll("document.querySelector('img.photo')?.naturalWidth===2")
        self.page.locator('#close').click();self.confirm();self.assertIn('Turno liberado',self.page.locator('#status').inner_text())
    def test_03_users_approval_and_account(self):
        self.create();self.click('[data-action=register]')
        self.page.locator('[name=username]').fill('testuser');self.page.locator('[name=display_name]').fill('Usuario ficticio');self.page.locator('[name=password]').fill('test-password')
        self.submit('register');self.login('testuser','test-password');self.assertIn('no está aprobada',self.page.locator('#status').inner_text())
        self.login();self.navigate('admin');f=self.page.locator('form[data-kind=user]').filter(has_text='Usuario ficticio');f.locator('[name=status]').select_option('approved');f.locator('[name=reset_password]').fill('approved-password');f.locator('button').click();self.idle()
        self.click('#nav [data-action=logout]');self.login('testuser','approved-password');self.assertEqual(self.page.locator('#nav [data-action=admin]').count(),0)
        self.navigate('account');self.page.locator('[name=display_name]').fill('Nombre actualizado');self.page.locator('[name=current_password]').fill('approved-password');self.page.locator('[name=password]').fill('replacement-password');self.submit('account')
        self.assertEqual(self.data()['users'][-1]['display_name'],'Nombre actualizado')
        self.click('#nav [data-action=logout]');self.login('testuser','replacement-password');self.assertTrue(self.page.locator('#nav').is_visible())
    def test_04_conflicting_turn_and_external_write(self):
        self.create();self.login()
        second=self.context.new_page();second.goto(self.base+'/ABRIR.html');second.locator('#accept').check();second.locator('#connect').click();self.poll("!document.body.hasAttribute('aria-busy')",second)
        self.assertIn('Otra sesión',second.locator('#status').inner_text())
        data=self.data();data['unknown_future_field']={'keep':'unchanged'};self.write_data(data)
        self.navigate('divers');self.click('[data-action=diverForm]');self.page.locator('[name=id]').fill('LATE');self.page.locator('[name=name]').fill('No debe guardarse');self.submit('diver')
        self.assertIn('han cambiado',self.page.locator('#status').inner_text());self.assertEqual(self.data()['divers'],[]);self.assertIn('unknown_future_field',self.data())
    def test_05_xlsx_duplicate_import(self):
        self.create();self.login();self.navigate('admin')
        fixture=REPO/'reference/source_materials/CONTROL BUCEADORES.xlsx'
        packed=io.BytesIO()
        with zipfile.ZipFile(fixture) as source,zipfile.ZipFile(packed,'w',zipfile.ZIP_DEFLATED) as target:
            for n in source.namelist():target.writestr(n,source.read(n))
        for _ in range(2):
            self.page.locator('[name=excel]').set_input_files({'name':'test.xlsx','mimeType':'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet','buffer':packed.getvalue()});self.submit('import');self.navigate('admin')
        self.assertEqual(len(self.data()['divers']),11);self.assertEqual(len(self.data()['dives']),15)
    def test_06_backup_rejects_corruption_and_unsafe_paths(self):
        def evaluate(blob):return self.page.evaluate("async b=>{try{await CBArchive.validateBackup(Uint8Array.from(b));return 'accepted'}catch(e){return e.message}}",list(blob))
        result=self.page.evaluate("async()=>{const s=await CBModel.fresh();return Array.from(await CBArchive.backup({'data/store.json':JSON.stringify(s),'config.json':'{}'},'test'))}")
        self.assertEqual(evaluate(bytes(result)),'accepted')
        with zipfile.ZipFile(io.BytesIO(bytes(result))) as z:files={n:z.read(n) for n in z.namelist()}
        packed=io.BytesIO()
        with zipfile.ZipFile(packed,'w',zipfile.ZIP_DEFLATED) as target:
            for n,v in files.items():target.writestr(n,v)
        self.assertEqual(evaluate(packed.getvalue()),'accepted')
        variants=[]
        for kind in ['checksum','path','unhashed','missing','badjson']:
            data=files.copy()
            if kind=='checksum':data['config.json']=b'{"changed":true}'
            if kind=='path':data['data/photos/../../outside.txt']=b'bad'
            if kind=='unhashed':data['data/photos/extra.png']=b'bad'
            if kind=='missing':del data['config.json']
            if kind=='badjson':data['data/store.json']=b'{' 
            b=io.BytesIO()
            with zipfile.ZipFile(b,'w',zipfile.ZIP_DEFLATED) as z:
                for n,v in data.items():z.writestr(n,v)
            self.assertNotEqual(evaluate(b.getvalue()),'accepted',kind)
    def test_07_legacy_recovery_is_explicit_and_backed_up(self):
        data=json.loads((REPO/'reference/v7_binary/store_v7.json').read_text())
        self.write_data(data);self.begin();self.assertEqual(self.data()['users'][0]['hash_iterations'] if 'hash_iterations' in self.data()['users'][0] else 0,0)
        self.click('[data-action=legacyRecovery]');self.page.locator('[name=password]').fill('recovered-password');self.page.locator('[name=repeat]').fill('recovered-password');self.page.locator('[name=authorized]').check()
        self.page.locator('form[data-kind=legacyRecovery] button').click();self.confirm();self.login('admin','recovered-password')
        self.assertEqual(len(self.data()['divers']),11)
        self.assertEqual(self.data()['users'][0]['hash_iterations'],120000)
        count=self.page.evaluate("async()=>{const root=await navigator.storage.getDirectory();const d=await(await root.getDirectoryHandle('data')).getDirectoryHandle('backups');let n=0;for await(const x of d.values())n++;return n}")
        self.assertGreater(count,0)
    def test_08_failed_write_keeps_old_data_and_can_recover(self):
        self.create();self.login()
        self.page.evaluate("""()=>{const original=FileSystemFileHandle.prototype.createWritable;FileSystemFileHandle.prototype.createWritable=function(...args){if(this.name==='store.json')throw new DOMException('Simulated disk failure','NotAllowedError');return original.apply(this,args)}}""")
        self.navigate('divers');self.click('[data-action=diverForm]');self.page.locator('[name=id]').fill('FAILED');self.page.locator('[name=name]').fill('Should not be saved');self.submit('diver')
        self.assertIn('Guardado no completado',self.page.locator('#status').inner_text());self.assertEqual(self.data()['divers'],[])
        self.page.locator('#close').click();self.confirm()
        fresh=self.context.new_page();self.page=fresh;fresh.goto(self.base+'/ABRIR.html');fresh.locator('#accept').check();fresh.locator('#connect').click();fresh.locator('#confirmation').wait_for(state='visible');self.confirm()
        self.login();self.assertEqual(self.data()['divers'],[])
        exists=self.page.evaluate("async()=>{try{const d=await(await navigator.storage.getDirectory()).getDirectoryHandle('data');await d.getFileHandle('browser-recovery.json');return true}catch(e){return false}}")
        self.assertFalse(exists)
    def test_09_invalid_store_is_never_reinitialized(self):
        self.page.evaluate("async()=>{const d=await(await navigator.storage.getDirectory()).getDirectoryHandle('data',{create:true});const w=await(await d.getFileHandle('store.json',{create:true})).createWritable();await w.write('{broken');await w.close()}")
        self.begin();self.assertTrue(self.page.locator('#status.error').is_visible());self.assertEqual(self.page.locator('[data-action=create]').count(),0)
        raw=self.page.evaluate("async()=>{const d=await(await navigator.storage.getDirectory()).getDirectoryHandle('data');return(await(await d.getFileHandle('store.json')).getFile()).text()}")
        self.assertEqual(raw,'{broken')
    def test_10_real_file_page_loads_without_server(self):
        # Separate page without substitution: confirms the shipped file:// page,
        # its scripts, secure-context crypto and API presence, not the OS picker.
        c=self.browser.new_context();p=c.new_page();errs=[];p.on('pageerror',lambda e:errs.append(str(e)));tmp=tempfile.TemporaryDirectory(prefix='Control Buceadores con espacios ');local=pathlib.Path(tmp.name);spec=importlib.util.spec_from_file_location('browser_packager',REPO/'scripts/package_browser.py');packager=importlib.util.module_from_spec(spec);spec.loader.exec_module(packager)
        with contextlib.redirect_stdout(io.StringIO()):package=packager.build(local/'package')
        with zipfile.ZipFile(package) as z:z.extractall(local/'unpacked')
        local=local/'unpacked/CONTROL_BUCEADORES_NAVEGADOR'
        try:
            try: p.goto((local/'ABRIR.html').as_uri())
            except PlaywrightError as e:
                if os.name!='nt' and 'ERR_BLOCKED_BY_ADMINISTRATOR' in str(e):self.skipTest('Managed local Chromium blocks file URLs; verify this case on the native Windows runner')
                raise
            self.assertIn('Seleccionar carpeta',p.locator('#connect').inner_text())
            self.assertTrue(p.evaluate('!!window.CBModel && !!window.CBArchive && !!crypto.subtle && isSecureContext && typeof showDirectoryPicker==="function"'))
            self.assertEqual(errs,[])
        finally:c.close();tmp.cleanup()

if __name__=='__main__':unittest.main(verbosity=2)
