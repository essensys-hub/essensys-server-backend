Tu est un architecte en IOT:

tu doit definit un architectre pour tester le server go du point vue fontionnel et performance.

tu peux me faire un client simulateur dasn le folder simulation.  il doit avoir un interface web  en react qui permet de de voir la tabel de reference et les 20 dernireè valeur. Il doit simuler le client essensys legacy. il doit permete de respecter sccification detailler dans le code server.go.

il doit avoir un backend qui appel comme le client essensys legacy à tous x secondes appel le server go et recupere les valeur et les stocke dans une base de donnes en local.

port server web: 5375


voici les doc de ref:

client-essensys-legacy/docs/protocol/tcp-single-packet.md 
client-essensys-legacy/docs/protocol/http-legacy-protocol.md
client-essensys-legacy/docs/protocol/exchange-table.md 
client-essensys-legacy/docs/server/api-endpoints.md 

il faut aussi rajouter un une fonctionnalité qui permet de simuler une flote de client essensys. 

Il peut avoir au max 100 client essensys. on doit pourvoir monter en charge par 5 clinets. 
On doit pouvoir faire des scenario de test avec des clients essensys sur les valeur table de reference.

Chque client doit avoir un identifiant unique et numeros de serie il faut voir dans le code du client legacy pour voir comment il est genere.

