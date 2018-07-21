import Router from 'https://unpkg.com/@nicolasparada/router@0.6.0/router.js';
import { guard } from './auth.js';
import { importWithCache } from './dynamic-import.js';

const router = new Router()
router.handle('/', guard(view('home'), view('access')))
router.handle('/callback', view('callback'))
router.handle(/^\/conversations\/([^\/]+)$/, guard(view('conversation'), view('access')))
router.handle(/^\//, view('not-found'))
router.install(render)

function view(pageName) {
    return (...args) => importWithCache(`/pages/${pageName}-page.js`)
        .then(m => m.default(...args))
}

async function render(resultPromise) {
    document.body.innerHTML = ''
    const result = await resultPromise
    if (result instanceof Node) {
        document.body.appendChild(result)
    }
}
