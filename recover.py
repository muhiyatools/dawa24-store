# -*- coding: utf-8 -*-
import re, subprocess, sys, io, os
ARCH=r'\u0600-\u06FF\u0750-\u077F\u060C\u061B\u061F\u0640'
def strip_variable(s):
    """Remove Arabic runs AND ?-runs, leaving only the invariant scaffolding."""
    s=re.sub(r'[?]{2,}','\x00',s)
    s=re.sub(r'['+ARCH+r']+','\x00',s)
    s=re.sub(r'[\x00\s]+','\x00',s)
    return s.strip()
def git_show(ref,p):
    r=subprocess.run(['git','show',f'{ref}:{p}'],capture_output=True)
    return r.stdout.decode('utf-8','replace') if r.returncode==0 else ''
EXTRA={n:['internal/ui/pages/admin_catalog_inventory.templ'] for n in
  ['admin_product_detail.templ','admin_warehouses.templ','admin_saving_products.templ',
   'admin_stocks.templ','admin_temp_warehouses.templ']}
REFS=['daca55c','3107422','f335edc','da48c82','8ec3e9b','0dbb55d','b0f791f']
def recover(path):
    cur=io.open(path,encoding='utf-8').read().split('\n')
    bad=[i for i,l in enumerate(cur) if re.search(r'\?{2,}',l)]
    if not bad: return 0,0
    gmap={}
    for ref in REFS:
        for sp in [path]+EXTRA.get(os.path.basename(path),[]):
            g=git_show(ref,sp)
            if not g: continue
            for gl in g.split('\n'):
                if re.search(r'['+ARCH+r']',gl) and not re.search(r'\?{2,}',gl):
                    gmap.setdefault(strip_variable(gl),gl)
    n=0
    for i in bad:
        key=strip_variable(cur[i])
        g=gmap.get(key)
        # require the scaffolding to be non-trivial, so a bare "???" line
        # cannot match an arbitrary other bare Arabic line
        if g is not None and len(key)>3:
            indent=re.match(r'[\t ]*',cur[i]).group(0)
            cur[i]=indent+g.lstrip(); n+=1
    if n: io.open(path,'w',encoding='utf-8').write('\n'.join(cur))
    return n,len(bad)-n
tot=0;left=0
for f in sys.argv[1:]:
    a,b=recover(f); tot+=a; left+=b
    if a or b: print(f"{os.path.basename(f):38} recovered={a:4} left={b}")
print("TOTAL recovered:",tot," still corrupt lines:",left)
