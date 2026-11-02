import {cp,rm,access} from 'node:fs/promises';
const source=new URL('../dist/',import.meta.url);
const target=new URL('../../../internal/pacaroute/web/',import.meta.url);
await access(new URL('index.html',source));
await rm(target,{recursive:true,force:true});
await cp(source,target,{recursive:true});
console.log('Embedded frontend copied to internal/pacaroute/web');
