import assert from "node:assert/strict";
import test from "node:test";
import { act } from "react";
import { JSDOM } from "jsdom";
import { createServer } from "vite";

for (const agent of ["codex", "opencode", "hermes", "configured"]) test(`project composer keeps models and supported modes for ${agent}`, async context => {
 const legacySupported = agent === "codex" || agent === "configured";
 const dom = new JSDOM('<div id="root"></div>', {url:"http://localhost/#/repositories"});
 const previous = new Map();
 for (const name of ["window","document","navigator","localStorage","Event","MouseEvent","fetch","IS_REACT_ACT_ENVIRONMENT"]) {
  previous.set(name,Object.getOwnPropertyDescriptor(globalThis,name));
  Object.defineProperty(globalThis,name,{configurable:true,writable:true,value:dom.window[name]});
 }
 globalThis.IS_REACT_ACT_ENVIRONMENT = true;
 const status = {state_backend:agent === "configured" ? "legacy" : "local",csrf_token:"csrf",jobs:[],workers:[],commands:[],workflows:["factory"],repositories:["owner/first","owner/second"],onboarding:{required:false,selection:{host:"local",agent},project_defaults:{"owner/second":{recipe:legacySupported ? "single" : "factory",instructions:"Test this project"}}}};
 const taskMode = ["local", "github"].includes(status.state_backend);
 let submitted;
 const availableModels = agent === "configured" ? [{id: "test/default", label: "Default"}] : [{id: "provider/model-a", label: "Model A"}, {id: "provider/model-b", label: "Model B"}];
 globalThis.fetch = async (url, options) => {
   if (url === "/api/v1/models") return {ok:true,json:async()=>({agent,models:availableModels})};
  if (url === "/api/v1/jobs") { submitted = JSON.parse(options.body); return {ok:true,json:async()=>({id:"new-job"})}; }
  return {ok:true,json:async()=>status};
 };
 const server = await createServer({server:{middlewareMode:true,hmr:false},appType:"custom"});
 let root;
 context.after(async()=>{await act(()=>root?.unmount());await server.close();dom.window.close();for(const [name,descriptor] of previous){if(descriptor)Object.defineProperty(globalThis,name,descriptor);else delete globalThis[name];}});
 await act(async()=>{root=(await server.ssrLoadModule("/src/main.jsx")).appRoot;});
 await act(()=>[...document.querySelectorAll(".project-card")][1].querySelector(".project-work button").click());
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,20));});
 const composer = document.querySelector(".run-composer");
 const selects = composer.querySelectorAll("select");
  assert.equal(selects[0].value,"owner/second");
  if (taskMode) {
   assert.match(selects[1].selectedOptions[0].textContent,legacySupported ? /Single agent/ : /Native agents/);
   assert.equal(selects[1].querySelector('option[value="factory"]').disabled, !legacySupported);
   assert.equal(selects[1].querySelector('option[value="single"]').disabled, false);
   assert.equal(selects[1].querySelector('option[value="native-v1"]').disabled, false);
  }
 const model = composer.querySelector('input[aria-label="Model"]');
 assert.ok(model, "task submissions expose a model input with selectable suggestions");
 assert.equal(model.value, "");
 const modelList = document.getElementById(model.getAttribute("list"));
 assert.deepEqual([...modelList.options].map(option => option.value), availableModels.map(option => option.id));
 for (const mode of taskMode ? (legacySupported ? ["native-v1", "single", "factory"] : ["native-v1", "single"]) : [""]) {
  if (taskMode) await act(()=>{selects[1].value=mode;selects[1].dispatchEvent(new dom.window.Event("change",{bubbles:true}));});
  assert.ok(composer.contains(model), `model selection remains available in ${mode || "legacy run"}`);
 }
 const areas = composer.querySelectorAll("textarea");
 if (taskMode) assert.equal(areas[1].value,"Test this project");
 function fill(element,value) {
  const setter=Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype,"value").set;
  setter.call(element,value); element.dispatchEvent(new dom.window.Event("input",{bubbles:true}));
 }
 await act(()=>{
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype,"value").set.call(model,taskMode ? "provider/model-b" : "custom/model-id");
  model.dispatchEvent(new dom.window.Event("input",{bubbles:true}));
 });
 if (taskMode) await act(()=>{selects[1].value=legacySupported ? "factory" : "single";selects[1].dispatchEvent(new dom.window.Event("change",{bubbles:true}));});
 await act(()=>fill(areas[0],"Implement the change"));
 await act(()=>composer.querySelector("form").dispatchEvent(new dom.window.Event("submit",{bubbles:true,cancelable:true})));
 assert.equal(submitted.repository,"owner/second");
 if (taskMode) {
  assert.equal(submitted.recipe,legacySupported ? "factory" : "single");
  assert.equal(submitted.project_instructions,"Test this project");
 } else {
  assert.equal(submitted.recipe,undefined);
  assert.equal(submitted.project_instructions,undefined);
 }
 assert.equal(submitted.model,taskMode ? "provider/model-b" : "custom/model-id");
 assert.equal(submitted.spec,"Implement the change");
});
