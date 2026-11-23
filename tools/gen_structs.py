#!/usr/bin/env python3
"""Генератор Go-структур из SQL-схем старого движка (DCMS-Social).
Использование: python3 tools/gen_structs.py [--all] table1 table2 ...
Читает /home/claude/tables.json (результат разбора install/db_tables/*.sql).
Правила типов: int*/bigint -> int64; varchar/text/enum/set -> string;
enum/set только из '0','1' -> bool (кроме pol). Поле id заменяется встроенным store.Base.
Имя поля: id_user -> IDUser, ank_mail -> AnkMail; json-тег = исходное имя колонки.
"""
import json,sys,re
T=json.load(open('/home/claude/tables.json'))
def pascal(s):
    p=[]
    for w in s.split('_'):
        p.append('ID' if w=='id' else w[:1].upper()+w[1:])
    return ''.join(p)
def gotype(col,typ,args):
    if typ in('int','bigint','tinyint','smallint','mediumint','integer'): return 'int64'
    if typ in('enum','set'):
        vals=[v.strip("' ") for v in args.split(',')]
        if set(vals)<= {'0','1'} and col!='pol': return 'bool'
        if all(re.fullmatch(r'\d+',v) for v in vals): return 'int64'
        return 'string'
    return 'string'
def gen(name):
    cols=T[name]; out=[f"// {pascal(name)} — таблица `{name}` старого движка.",f"type {pascal(name)} struct {{","\tstore.Base"]
    seen=set()
    for c,t,a in cols:
        if c=='id' or c in seen: continue
        seen.add(c)
        out.append(f"\t{pascal(c)} {gotype(c,t,a)} `json:\"{c}\"`")
    out.append("}")
    return '\n'.join(out)
if __name__=='__main__':
    names=sys.argv[1:]
    if names==['--all']: names=list(T)
    for n in names:
        if n not in T: print('// unknown table',n); continue
        print(gen(n)); print()
