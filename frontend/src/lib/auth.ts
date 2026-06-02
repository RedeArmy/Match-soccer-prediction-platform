import { auth } from '@clerk/nextjs/server'

// Returns the Clerk JWT for server-side BFF requests.
// Throws when called outside an authenticated context.
export async function getServerToken(): Promise<string | null> {
  const { getToken } = await auth()
  return getToken()
}
