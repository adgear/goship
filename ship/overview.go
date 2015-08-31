package ship

var htmlOverview = `<html>
<body>
 <table>
 {{range .Builders}}<tr><td>{{.Name}}</td><td>{{.Request.User}}</td><td>{{.Request.When}}</tr>{{end}}
 </table>
</body>
</html>`
