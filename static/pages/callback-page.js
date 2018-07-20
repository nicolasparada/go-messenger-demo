import { navigate } from 'https://unpkg.com/@nicolasparada/router@0.6.0/router.js';
import http from '../http.js';

export default async function callbackPage() {
    const url = new URL(location.toString())
    const token = url.searchParams.get('token')
    const expiresAt = url.searchParams.get('expires_at')

    if (token === null || expiresAt === null) {
        alert('Invalid URL')
        navigate('/', true)
        return
    }

    const authUser = await getAuthUser(token)

    localStorage.setItem('auth_user', JSON.stringify(authUser))
    localStorage.setItem('token', token)
    localStorage.setItem('expires_at', expiresAt)

    location.replace('/')
}

function getAuthUser(token) {
    return http.get('/api/auth_user', { authorization: `Bearer ${token}` })
}
