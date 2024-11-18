import { AuthenticationStatus, ConnectButton } from '@rainbow-me/rainbowkit';
import './App.css';
import { useAccount } from 'wagmi';
import { useEffect, useState } from 'react';
import { RainbowKitProvider, createAuthenticationAdapter, RainbowKitAuthenticationProvider } from '@rainbow-me/rainbowkit';
import logo from './assets/whole_logo.png';
import { LoginWithDimo } from '@dimo-network/login-with-dimo';

class DIMODexMessage {
  state?: string;
  challenge?: string;
  constructor(param: Partial<DIMODexMessage>) {
    this.state = param.state;
    this.challenge = param.challenge;
  }
}

interface AuthData {
  token: string;
}


function App() {
  const [status, setStatus] = useState<AuthenticationStatus>("unauthenticated");
  const account = useAccount();

  const authenticationAdapter = createAuthenticationAdapter({
    getNonce: async () => {
      const address = account?.address;
      const response = await fetch(`${import.meta.env.DIMO_API_BASEURL}/auth/web3/generate_challenge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address }),
      });

      if (!response.ok) {
        throw new Error('Failed to fetch nonce');
      }

      const challengeResponse: string = await response.text();
      return challengeResponse;
    },

    createMessage: ({ nonce }) => {
      const message: { state: string; challenge: string } = JSON.parse(nonce);
      return new DIMODexMessage({
        state: message.state,
        challenge: message.challenge,
      });
    },

    verify: async ({ message, signature }) => {
      const verifyRes = await fetch(`${import.meta.env.DIMO_API_BASEURL}/auth/web3/submit_challenge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ state: message.state, signature }),
      });

      if (!verifyRes.ok) {
        throw new Error('Failed to verify signature');
      }

      setStatus('authenticated');
      return verifyRes.ok;
    },

    signOut: async () => {
      await fetch('/api/logout');
    },
  });

  useEffect(() => {
    if (status === 'authenticated') {
      // Redirect to vehicles page after successful session establishment
      window.location.href = `${import.meta.env.DIMO_API_BASEURL}/vehicles/me`;
    }
  }, [status]);

  const clientId = import.meta.env.DIMO_CLIENT_ID;
  const redirectUri = import.meta.env.DIMO_REDIRECT_URI;
  const environment = import.meta.env.DIMO_ENVIRONMENT;
  const permissionTemplateId = import.meta.env.DIMO_PERMISSION_TEMPLATE_ID;

  return (
      <RainbowKitAuthenticationProvider adapter={authenticationAdapter} status={status}>
        <RainbowKitProvider>
          <>
            <div className="logo-container">
              <img src={logo} alt="Logo" className="logo" />
            </div>
            <div className="connect-button-container">
              <ConnectButton />
            </div>
            <div className="connect-button-container">
              <p><a href="/login-jwt">Login with JWT</a></p>
            </div>
            <div className="connect-button-container">
              <LoginWithDimo
                  mode="popup"
                  clientId={clientId}
                  redirectUri={redirectUri}
                  environment={environment}
                  permissionTemplateId={permissionTemplateId}
                  onSuccess={(authData: AuthData) => {
                    console.log("JWT received:", authData.token);

                    // Send the JWT to the backend to establish the session
                    fetch('/login-jwt', {
                      method: 'POST',
                      headers: {
                        'Content-Type': 'application/json',
                      },
                      credentials: 'include',
                      body: JSON.stringify({ jwt: authData.token }),
                    })
                        .then(response => {
                          if (response.ok) {
                            console.log("Session established, redirecting to /vehicles/me");
                            setStatus('authenticated');
                          } else {
                            console.error("Failed to establish session, response:", response);
                          }
                        })
                        .catch(error => {
                          console.error("Error sending JWT to backend:", error);
                        });
                  }}
                  onError={(error: Error) => {
                    console.error("Authentication error:", error);
                  }}
              />
            </div>
          </>
        </RainbowKitProvider>
      </RainbowKitAuthenticationProvider>
  );
}

export default App;
