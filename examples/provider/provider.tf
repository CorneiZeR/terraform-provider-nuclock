terraform {
  required_providers {
    nuclock = {
      source = "corneizer/nuclock"
    }
  }
}

# The address and a service key, or NUCLOCK_URL and NUCLOCK_API_KEY.
provider "nuclock" {
  url = "https://api.clock.example.com"
}
